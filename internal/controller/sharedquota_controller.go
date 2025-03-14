/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	quotav1 "caih.com/api/v1"
	quotapkg "caih.com/pkg/quota"
	evaluatorcore "caih.com/pkg/quota/evaluator/core"
	"caih.com/pkg/quota/generic"
	"caih.com/pkg/quota/install"
)

const (
	controllerName                 = "sharedquota"
	DefaultResyncPeriod            = 5 * time.Minute
	DefaultMaxConcurrentReconciles = 8
)

var _ reconcile.Reconciler = &SharedQuotaReconciler{}

// SharedQuotaReconciler reconciles a SharedQuota object
type SharedQuotaReconciler struct {
	client.Client
	logger   logr.Logger
	recorder record.EventRecorder
	Scheme   *runtime.Scheme
	// Knows how to calculate usage
	registry                quotapkg.Registry
	MaxConcurrentReconciles int
	// Controls full recalculation of quota usage
	ResyncPeriod time.Duration
}

func (r *SharedQuotaReconciler) Name() string {
	return controllerName
}

// SetupWithManager sets up the controller with the Manager.
func (r *SharedQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = ctrl.Log.WithName("controllers").WithName(controllerName)
	r.recorder = mgr.GetEventRecorderFor(controllerName)
	r.registry = generic.NewRegistry(install.NewQuotaConfigurationForControllers(mgr.GetClient()).Evaluators())
	r.MaxConcurrentReconciles = DefaultMaxConcurrentReconciles
	r.ResyncPeriod = DefaultResyncPeriod
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&quotav1.SharedQuota{}).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: DefaultMaxConcurrentReconciles,
		}).
		WithEventFilter(predicate.GenerationChangedPredicate{
			TypedFuncs: predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldQuota := e.ObjectOld.(*quotav1.SharedQuota)
					newQuota := e.ObjectNew.(*quotav1.SharedQuota)
					return !equality.Semantic.DeepEqual(oldQuota.Spec, newQuota.Spec)
				},
			},
		}).
		Build(r)
	if err != nil {
		return err
	}

	resources := []client.Object{
		&corev1.Pod{},
		&corev1.Service{},
		&corev1.PersistentVolumeClaim{},
	}
	realClock := clock.RealClock{}
	for _, resource := range resources {
		p := predicate.GenerationChangedPredicate{
			TypedFuncs: predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool {
					return false
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					notifyChange := false
					// we only want to queue the updates we care about though as too much noise will overwhelm queue.
					switch e.ObjectOld.(type) {
					case *corev1.Pod:
						oldPod := e.ObjectOld.(*corev1.Pod)
						newPod := e.ObjectNew.(*corev1.Pod)
						notifyChange = evaluatorcore.QuotaV1Pod(oldPod, realClock) && !evaluatorcore.QuotaV1Pod(newPod, realClock)
					case *corev1.Service:
						oldService := e.ObjectOld.(*corev1.Service)
						newService := e.ObjectNew.(*corev1.Service)
						notifyChange = evaluatorcore.GetQuotaServiceType(oldService) != evaluatorcore.GetQuotaServiceType(newService)
					case *corev1.PersistentVolumeClaim:
						notifyChange = true
					}
					return notifyChange
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return true
				},
			},
		}
		if err = c.Watch(source.Kind(mgr.GetCache(), resource, handler.EnqueueRequestsFromMapFunc(r.mapper), p)); err != nil {
			return err
		}
	}
	return nil
}

func (r *SharedQuotaReconciler) mapper(ctx context.Context, h client.Object) []reconcile.Request {
	// check if the quota controller can evaluate this kind, if not, ignore it altogether...
	var result []reconcile.Request
	evaluators := r.registry.List()
	resourceQuotaNames, err := quotapkg.ResourceQuotaNamesFor(ctx, r.Client, h.GetNamespace())
	if err != nil {
		klog.Errorf("failed to get resource quota names for: %v %T %v, err: %v", h.GetNamespace(), h, h.GetName(), err)
		return result
	}
	// only queue those quotas that are tracking a resource associated with this kind.
	for _, resourceQuotaName := range resourceQuotaNames {
		resourceQuota := &quotav1.SharedQuota{}
		if err := r.Get(ctx, types.NamespacedName{Name: resourceQuotaName}, resourceQuota); err != nil {
			klog.Errorf("failed to get resource quota: %v, err: %v", resourceQuotaName, err)
			return result
		}
		resourceQuotaResources := quotapkg.ResourceNames(resourceQuota.Status.Total.Hard)
		for _, evaluator := range evaluators {
			matchedResources := evaluator.MatchingResources(resourceQuotaResources)
			if len(matchedResources) > 0 {
				result = append(result, reconcile.Request{NamespacedName: types.NamespacedName{Name: resourceQuotaName}})
				break
			}
		}
	}
	klog.V(6).Infof("resource quota reconcile after resource change: %v %T %v, %+v", h.GetNamespace(), h, h.GetName(), result)
	return result
}

// +kubebuilder:rbac:groups=quota.caih.com,resources=sharedquotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=quota.caih.com,resources=sharedquotas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=quota.caih.com,resources=sharedquotas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SharedQuota object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *SharedQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("sharedquota", req.NamespacedName)
	rootCtx := klog.NewContext(ctx, logger)
	sharedQuota := &quotav1.SharedQuota{}
	if err := r.Get(rootCtx, req.NamespacedName, sharedQuota); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.syncQuotaForNamespaces(sharedQuota); err != nil {
		logger.Error(err, "failed to sync quota")
		return ctrl.Result{}, err
	}

	r.recorder.Event(sharedQuota, corev1.EventTypeNormal, "Synced", "Synced successfully")
	return ctrl.Result{RequeueAfter: r.ResyncPeriod}, nil
}

func (r *SharedQuotaReconciler) syncQuotaForNamespaces(originalQuota *quotav1.SharedQuota) error {
	quota := originalQuota.DeepCopy()
	ctx := context.TODO()
	// get the list of namespaces that match this cluster quota
	matchingNamespaceList := corev1.NamespaceList{}
	if err := r.List(ctx, &matchingNamespaceList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(quota.Spec.LabelSelector)}); err != nil {
		return err
	}

	if quota.Status.Namespaces == nil {
		quota.Status.Namespaces = make([]quotav1.ResourceQuotaStatusByNamespace, 0)
	}

	matchingNamespaceNames := make([]string, 0)
	for _, namespace := range matchingNamespaceList.Items {
		matchingNamespaceNames = append(matchingNamespaceNames, namespace.Name)
	}

	for _, namespace := range matchingNamespaceList.Items {
		namespaceName := namespace.Name
		namespaceTotals, _ := quotapkg.GetResourceQuotasStatusByNamespace(quota.Status.Namespaces, namespaceName)

		actualUsage, err := quotaUsageCalculationFunc(namespaceName, quota.Spec.Quota.Scopes, quota.Spec.Quota.Hard, r.registry, quota.Spec.Quota.ScopeSelector)
		if err != nil {
			return err
		}
		recalculatedStatus := corev1.ResourceQuotaStatus{
			Used: actualUsage,
			Hard: quota.Spec.Quota.Hard,
		}

		// subtract old usage, add new usage
		quota.Status.Total.Used = quotapkg.Subtract(quota.Status.Total.Used, namespaceTotals.Used)
		quota.Status.Total.Used = quotapkg.Add(quota.Status.Total.Used, recalculatedStatus.Used)
		quotapkg.InsertResourceQuotasStatus(&quota.Status.Namespaces, quotav1.ResourceQuotaStatusByNamespace{
			Namespace:           namespaceName,
			ResourceQuotaStatus: recalculatedStatus,
		})
	}

	// Remove any namespaces from quota.status that no longer match.
	statusCopy := quota.Status.Namespaces.DeepCopy()
	for _, namespaceTotals := range statusCopy {
		namespaceName := namespaceTotals.Namespace
		if !slices.Contains(matchingNamespaceNames, namespaceName) {
			quota.Status.Total.Used = quotapkg.Subtract(quota.Status.Total.Used, namespaceTotals.Used)
			quotapkg.RemoveResourceQuotasStatusByNamespace(&quota.Status.Namespaces, namespaceName)
		}
	}

	quota.Status.Total.Hard = quota.Spec.Quota.Hard

	// if there's no change, no update, return early.  NewAggregate returns nil on empty input
	if equality.Semantic.DeepEqual(quota, originalQuota) {
		return nil
	}

	klog.V(6).Infof("update resource quota: %+v", quota)
	if err := r.Status().Update(ctx, quota); err != nil {
		return err
	}

	return nil
}

// quotaUsageCalculationFunc is a function to calculate quota usage.  It is only configurable for easy unit testing
// NEVER CHANGE THIS OUTSIDE A TEST
var quotaUsageCalculationFunc = quotapkg.CalculateUsage
