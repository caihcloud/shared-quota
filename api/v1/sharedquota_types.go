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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SharedQuotaSpec defines the desired state of SharedQuota.
type SharedQuotaSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// LabelSelector is used to select projects by label.
	LabelSelector map[string]string `json:"selector" protobuf:"bytes,1,opt,name=selector"`

	// Quota defines the desired quota
	Quota corev1.ResourceQuotaSpec `json:"quota" protobuf:"bytes,2,opt,name=quota"`
}

// SharedQuotaStatus defines the observed state of SharedQuota.
type SharedQuotaStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Total defines the actual enforced quota and its current usage across all projects
	Total corev1.ResourceQuotaStatus `json:"total" protobuf:"bytes,1,opt,name=total"`

	// Namespaces slices the usage by project.
	Namespaces ResourceQuotasStatusByNamespace `json:"namespaces" protobuf:"bytes,2,rep,name=namespaces"`
}

// ResourceQuotasStatusByNamespace bundles multiple ResourceQuotaStatusByNamespace
type ResourceQuotasStatusByNamespace []ResourceQuotaStatusByNamespace

// ResourceQuotaStatusByNamespace gives status for a particular project
type ResourceQuotaStatusByNamespace struct {
	corev1.ResourceQuotaStatus `json:",inline"`

	// Namespace the project this status applies to
	Namespace string `json:"namespace" protobuf:"bytes,1,opt,name=namespace"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// SharedQuota is the Schema for the sharedquotas API.
type SharedQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SharedQuotaSpec   `json:"spec,omitempty"`
	Status SharedQuotaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SharedQuotaList contains a list of SharedQuota.
type SharedQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SharedQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SharedQuota{}, &SharedQuotaList{})
}
