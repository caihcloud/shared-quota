/*
 * Please refer to the LICENSE file in the root directory of the project.
 * https://github.com/kubesphere/kubesphere/blob/master/LICENSE
 */

package quota

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	quotav1 "caih.com/api/v1"
)

// Following code copied from github.com/openshift/library-go/pkg/quota/quotautil
func getResourceQuotasStatusByNamespace(namespaceStatuses quotav1.ResourceQuotasStatusByNamespace, namespace string) (corev1.ResourceQuotaStatus, bool) {
	for i := range namespaceStatuses {
		curr := namespaceStatuses[i]
		if curr.Namespace == namespace {
			return curr.ResourceQuotaStatus, true
		}
	}
	return corev1.ResourceQuotaStatus{}, false
}

func removeResourceQuotasStatusByNamespace(namespaceStatuses *quotav1.ResourceQuotasStatusByNamespace, namespace string) {
	newNamespaceStatuses := quotav1.ResourceQuotasStatusByNamespace{}
	for i := range *namespaceStatuses {
		curr := (*namespaceStatuses)[i]
		if curr.Namespace == namespace {
			continue
		}
		newNamespaceStatuses = append(newNamespaceStatuses, curr)
	}
	*namespaceStatuses = newNamespaceStatuses
}

func insertResourceQuotasStatus(namespaceStatuses *quotav1.ResourceQuotasStatusByNamespace, newStatus quotav1.ResourceQuotaStatusByNamespace) {
	newNamespaceStatuses := quotav1.ResourceQuotasStatusByNamespace{}
	found := false
	for i := range *namespaceStatuses {
		curr := (*namespaceStatuses)[i]
		if curr.Namespace == newStatus.Namespace {
			// do this so that we don't change serialization order
			newNamespaceStatuses = append(newNamespaceStatuses, newStatus)
			found = true
			continue
		}
		newNamespaceStatuses = append(newNamespaceStatuses, curr)
	}
	if !found {
		newNamespaceStatuses = append(newNamespaceStatuses, newStatus)
	}
	*namespaceStatuses = newNamespaceStatuses
}

func ResourceQuotaNamesFor(ctx context.Context, client client.Client, namespaceName string) ([]string, error) {
	namespace := &corev1.Namespace{}
	var resourceQuotaNames []string
	if err := client.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace); err != nil {
		return resourceQuotaNames, err
	}
	if len(namespace.Labels) == 0 {
		return resourceQuotaNames, nil
	}
	resourceQuotaList := &quotav1.SharedQuotaList{}
	if err := client.List(ctx, resourceQuotaList); err != nil {
		return resourceQuotaNames, err
	}
	for _, resourceQuota := range resourceQuotaList.Items {
		if len(resourceQuota.Spec.LabelSelector) > 0 &&
			labels.SelectorFromSet(resourceQuota.Spec.LabelSelector).Matches(labels.Set(namespace.Labels)) {
			resourceQuotaNames = append(resourceQuotaNames, resourceQuota.Name)
		}
	}
	return resourceQuotaNames, nil
}

// GetResourceQuotasStatusByNamespace
// Following code copied from github.com/openshift/library-go/pkg/quota/quotautil
func GetResourceQuotasStatusByNamespace(namespaceStatuses quotav1.ResourceQuotasStatusByNamespace, namespace string) (corev1.ResourceQuotaStatus, bool) {
	for i := range namespaceStatuses {
		curr := namespaceStatuses[i]
		if curr.Namespace == namespace {
			return curr.ResourceQuotaStatus, true
		}
	}
	return corev1.ResourceQuotaStatus{}, false
}

func RemoveResourceQuotasStatusByNamespace(namespaceStatuses *quotav1.ResourceQuotasStatusByNamespace, namespace string) {
	newNamespaceStatuses := quotav1.ResourceQuotasStatusByNamespace{}
	for i := range *namespaceStatuses {
		curr := (*namespaceStatuses)[i]
		if curr.Namespace == namespace {
			continue
		}
		newNamespaceStatuses = append(newNamespaceStatuses, curr)
	}
	*namespaceStatuses = newNamespaceStatuses
}

func InsertResourceQuotasStatus(namespaceStatuses *quotav1.ResourceQuotasStatusByNamespace, newStatus quotav1.ResourceQuotaStatusByNamespace) {
	newNamespaceStatuses := quotav1.ResourceQuotasStatusByNamespace{}
	found := false
	for i := range *namespaceStatuses {
		curr := (*namespaceStatuses)[i]
		if curr.Namespace == newStatus.Namespace {
			// do this so that we don't change serialization order
			newNamespaceStatuses = append(newNamespaceStatuses, newStatus)
			found = true
			continue
		}
		newNamespaceStatuses = append(newNamespaceStatuses, curr)
	}
	if !found {
		newNamespaceStatuses = append(newNamespaceStatuses, newStatus)
	}
	*namespaceStatuses = newNamespaceStatuses
}
