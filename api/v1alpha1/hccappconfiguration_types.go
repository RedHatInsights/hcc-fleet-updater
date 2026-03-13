/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageRegistryEntry identifies an image repository that an app uses.
type ImageRegistryEntry struct {
	// image is the full container image repository path (e.g. quay.io/cloudservices/advisor-backend).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
}

// HCCAppConfigurationSpec defines the desired state of HCCAppConfiguration.
type HCCAppConfigurationSpec struct {
	// clowdAppName is the metadata.name of the ClowdApp CR to monitor.
	// +kubebuilder:validation:MinLength=1
	ClowdAppName string `json:"clowdAppName"`

	// namespace is the namespace of the ClowdApp. Defaults to the operator namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// images lists the image repositories this app uses.
	// +optional
	Images []ImageRegistryEntry `json:"images,omitempty"`

	// enabled controls whether this app configuration is actively monitored.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// HCCAppConfigurationStatus defines the observed state of HCCAppConfiguration.
type HCCAppConfigurationStatus struct {
	// currentImages reflects the image:tag pairs currently running on the ClowdApp.
	// +optional
	CurrentImages []ImageTagPair `json:"currentImages,omitempty"`

	// lastUpdated is when the status was last refreshed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// healthy indicates whether the ClowdApp's deployments are healthy.
	// +optional
	Healthy *bool `json:"healthy,omitempty"`

	// conditions represent the current state of the HCCAppConfiguration.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ClowdApp",type="string",JSONPath=".spec.clowdAppName"
// +kubebuilder:printcolumn:name="Enabled",type="boolean",JSONPath=".spec.enabled"
// +kubebuilder:printcolumn:name="Healthy",type="boolean",JSONPath=".status.healthy"

// HCCAppConfiguration is the Schema for the hccappconfigurations API.
// It provides persistent inventory and health tracking for a ClowdApp.
type HCCAppConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec HCCAppConfigurationSpec `json:"spec"`

	// +optional
	Status HCCAppConfigurationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// HCCAppConfigurationList contains a list of HCCAppConfiguration.
type HCCAppConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []HCCAppConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HCCAppConfiguration{}, &HCCAppConfigurationList{})
}
