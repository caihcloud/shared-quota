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

package v1

import (
	"context"
	"net/http"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilwait "k8s.io/apimachinery/pkg/util/wait"
	admissionapi "k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/resourcequota"
	resourcequotaapi "k8s.io/apiserver/pkg/admission/plugin/resourcequota/apis/resourcequota"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"caih.com/pkg/quota"
	"caih.com/pkg/quota/generic"
	"caih.com/pkg/quota/install"
	"caih.com/pkg/scheme"
)

const (
	numEvaluatorThreads = 10
)

type SharedQuotaAdmission struct {
	client client.Client

	decoder webhook.AdmissionDecoder

	lockFactory LockFactory

	// these are used to create the evaluator
	registry quota.Registry

	init      sync.Once
	evaluator resourcequota.Evaluator
}

const webhookName = "shared-quota-webhook"

// SetupWithManager registers the webhook for Pod in the manager.
func SetupWithManager(mgr ctrl.Manager) error {
	sharedQuotaAdmission := &SharedQuotaAdmission{
		client:      mgr.GetClient(),
		lockFactory: NewDefaultLockFactory(),
		decoder:     admission.NewDecoder(mgr.GetScheme()),
		registry:    generic.NewRegistry(install.NewQuotaConfigurationForAdmission().Evaluators()),
	}
	mgr.GetWebhookServer().Register("/validate-quota-caih-com-v1", &webhook.Admission{Handler: sharedQuotaAdmission})
	return nil
}

// +kubebuilder:webhook:path=/validate-quota-caih-com-v1,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1

// PodCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Pod when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.

func (a *SharedQuotaAdmission) Handle(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
	// ignore all operations that correspond to sub-resource actions
	if len(req.RequestSubResource) != 0 {
		return webhook.Allowed("")
	}
	// ignore cluster level resources
	if len(req.Namespace) == 0 {
		return webhook.Allowed("")
	}

	a.init.Do(func() {
		resourceQuotaAccessor := quota.NewQuotaAccessor(a.client)
		a.evaluator = NewQuotaEvaluator(resourceQuotaAccessor, install.DefaultIgnoredResources(),
			a.registry, a.lockAquisition, &resourcequotaapi.Configuration{}, numEvaluatorThreads, utilwait.NeverStop)
	})

	attributesRecord, err := convertToAdmissionAttributes(req)
	if err != nil {
		klog.Error(err)
		return webhook.Errored(http.StatusBadRequest, err)
	}

	if err := a.evaluator.Evaluate(attributesRecord); err != nil {
		if errors.IsForbidden(err) {
			klog.Info(err)
			return webhook.Denied(err.Error())
		}
		klog.Error(err)
		return webhook.Errored(http.StatusInternalServerError, err)
	}

	return webhook.Allowed("")
}

type ByName []corev1.ResourceQuota

func (v ByName) Len() int           { return len(v) }
func (v ByName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v ByName) Less(i, j int) bool { return v[i].Name < v[j].Name }

func (a *SharedQuotaAdmission) lockAquisition(quotas []corev1.ResourceQuota) func() {
	var locks []sync.Locker

	// acquire the locks in alphabetical order because I'm too lazy to think of something clever
	sort.Sort(ByName(quotas))
	for _, q := range quotas {
		lock := a.lockFactory.GetLock(string(q.UID))
		lock.Lock()
		locks = append(locks, lock)
	}

	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Unlock()
		}
	}
}

func convertToAdmissionAttributes(req admission.Request) (admissionapi.Attributes, error) {
	var err error
	var object runtime.Object
	if len(req.Object.Raw) > 0 {
		object, _, err = scheme.Codecs.UniversalDeserializer().Decode(req.Object.Raw, nil, nil)
		if err != nil {
			return nil, err
		}
	}

	var oldObject runtime.Object
	if len(req.OldObject.Raw) > 0 {
		oldObject, _, err = scheme.Codecs.UniversalDeserializer().Decode(req.OldObject.Raw, nil, nil)
		if err != nil {
			klog.Error(err)
			return nil, err
		}
	}

	var operationOptions runtime.Object
	if len(req.Options.Raw) > 0 {
		operationOptions, _, err = scheme.Codecs.UniversalDeserializer().Decode(req.Options.Raw, nil, nil)
		if err != nil {
			klog.Error(err)
			return nil, err
		}
	}

	extras := map[string][]string{}
	for k, v := range req.UserInfo.Extra {
		extras[k] = v
	}

	attributesRecord := admissionapi.NewAttributesRecord(object,
		oldObject,
		schema.GroupVersionKind{
			Group:   req.RequestKind.Group,
			Version: req.RequestKind.Version,
			Kind:    req.RequestKind.Kind,
		},
		req.Namespace,
		req.Name,
		schema.GroupVersionResource{
			Group:    req.RequestResource.Group,
			Version:  req.RequestResource.Version,
			Resource: req.RequestResource.Resource,
		},
		req.SubResource,
		admissionapi.Operation(req.Operation),
		operationOptions,
		*req.DryRun,
		&user.DefaultInfo{
			Name:   req.UserInfo.Username,
			UID:    req.UserInfo.UID,
			Groups: req.UserInfo.Groups,
			Extra:  extras,
		})
	return attributesRecord, nil
}
