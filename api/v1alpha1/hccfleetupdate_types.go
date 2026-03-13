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

// FailurePolicy defines how the fleet update handles app failures.
// +kubebuilder:validation:Enum=Continue;Halt
type FailurePolicy string

const (
	FailurePolicyContinue FailurePolicy = "Continue"
	FailurePolicyHalt     FailurePolicy = "Halt"
)

// FleetUpdatePhase represents the overall phase of a fleet update.
// +kubebuilder:validation:Enum=Pending;InProgress;Completed;PartiallyFailed;Failed;Paused
type FleetUpdatePhase string

const (
	FleetUpdatePhasePending         FleetUpdatePhase = "Pending"
	FleetUpdatePhaseInProgress      FleetUpdatePhase = "InProgress"
	FleetUpdatePhaseCompleted       FleetUpdatePhase = "Completed"
	FleetUpdatePhasePartiallyFailed FleetUpdatePhase = "PartiallyFailed"
	FleetUpdatePhaseFailed          FleetUpdatePhase = "Failed"
	FleetUpdatePhasePaused          FleetUpdatePhase = "Paused"
)

// AppRolloutPhase represents the phase of an individual app rollout.
// +kubebuilder:validation:Enum=Pending;InProgress;Succeeded;Failed;Skipped
type AppRolloutPhase string

const (
	AppRolloutPhasePending    AppRolloutPhase = "Pending"
	AppRolloutPhaseInProgress AppRolloutPhase = "InProgress"
	AppRolloutPhaseSucceeded  AppRolloutPhase = "Succeeded"
	AppRolloutPhaseFailed     AppRolloutPhase = "Failed"
	AppRolloutPhaseSkipped    AppRolloutPhase = "Skipped"
)

// ImageTagPair specifies a container image and its desired tag.
type ImageTagPair struct {
	// image is the full container image repository path (e.g. quay.io/cloudservices/advisor-backend).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// tag is the desired image tag (e.g. a1b2c3d).
	// +kubebuilder:validation:MinLength=1
	Tag string `json:"tag"`
}

// AppImageSpec defines a ClowdApp and its desired image updates.
type AppImageSpec struct {
	// clowdAppName is the metadata.name of the target ClowdApp CR.
	// +kubebuilder:validation:MinLength=1
	ClowdAppName string `json:"clowdAppName"`

	// namespace is the namespace of the ClowdApp. Defaults to the operator namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// images is the list of image:tag pairs to apply to this ClowdApp.
	// +kubebuilder:validation:MinItems=1
	Images []ImageTagPair `json:"images"`

	// priority controls wave ordering. Lower values are processed first.
	// +kubebuilder:default=10
	// +optional
	Priority int32 `json:"priority,omitempty"`
}

// RolloutStrategy configures how the fleet update proceeds.
type RolloutStrategy struct {
	// maxParallelism is the maximum number of apps updated concurrently.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxParallelism int32 `json:"maxParallelism,omitempty"`

	// healthCheckTimeout is how long to wait for an app to become healthy after patching.
	// +kubebuilder:default="10m"
	// +optional
	HealthCheckTimeout *metav1.Duration `json:"healthCheckTimeout,omitempty"`

	// failurePolicy determines behavior when an app fails: Continue or Halt.
	// +kubebuilder:default=Continue
	// +optional
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`
}

// HCCFleetUpdateSpec defines the desired state of HCCFleetUpdate.
type HCCFleetUpdateSpec struct {
	// description is a human-readable note about this update (e.g. "March 2026 CVE refresh").
	// +optional
	Description string `json:"description,omitempty"`

	// apps is the list of ClowdApps and their desired image updates.
	// +kubebuilder:validation:MinItems=1
	Apps []AppImageSpec `json:"apps"`

	// strategy configures parallelism, timeouts, and failure handling.
	// +optional
	Strategy RolloutStrategy `json:"strategy,omitempty"`

	// paused halts the rollout without reverting any completed updates.
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// RolloutSummary contains aggregate counters for the fleet update.
type RolloutSummary struct {
	// total is the total number of apps in this fleet update.
	Total int32 `json:"total"`

	// pending is the number of apps not yet started.
	Pending int32 `json:"pending"`

	// inProgress is the number of apps currently being updated.
	InProgress int32 `json:"inProgress"`

	// succeeded is the number of apps that completed successfully.
	Succeeded int32 `json:"succeeded"`

	// failed is the number of apps that failed.
	Failed int32 `json:"failed"`

	// skipped is the number of apps that were skipped.
	Skipped int32 `json:"skipped"`
}

// AppRolloutStatus tracks the rollout status of a single app.
type AppRolloutStatus struct {
	// clowdAppName is the name of the ClowdApp.
	ClowdAppName string `json:"clowdAppName"`

	// namespace is the namespace of the ClowdApp.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// phase is the current rollout phase for this app.
	Phase AppRolloutPhase `json:"phase"`

	// message provides additional detail about the current phase.
	// +optional
	Message string `json:"message,omitempty"`

	// previousImages records the image:tag values before this update, for rollback reference.
	// +optional
	PreviousImages []ImageTagPair `json:"previousImages,omitempty"`

	// lastTransitionTime is when this app's phase last changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// HCCFleetUpdateStatus defines the observed state of HCCFleetUpdate.
type HCCFleetUpdateStatus struct {
	// phase is the overall phase of the fleet update.
	// +optional
	Phase FleetUpdatePhase `json:"phase,omitempty"`

	// autoPaused indicates the controller has halted processing due to a
	// FailurePolicy of Halt. The rollout will remain paused until the user
	// sets spec.paused=false or deletes the resource.
	// +optional
	AutoPaused bool `json:"autoPaused,omitempty"`

	// startTime is when the fleet update began processing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// completionTime is when the fleet update finished (success or failure).
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// summary contains aggregate rollout counters.
	// +optional
	Summary RolloutSummary `json:"summary,omitempty"`

	// appStatuses tracks the per-app rollout status.
	// +optional
	AppStatuses []AppRolloutStatus `json:"appStatuses,omitempty"`

	// conditions represent the current state of the HCCFleetUpdate.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Succeeded",type="integer",JSONPath=".status.summary.succeeded"
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.summary.failed"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.summary.total"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// HCCFleetUpdate is the Schema for the hccfleetupdates API.
// It orchestrates fleet-wide image rollouts across ClowdApp CRs.
type HCCFleetUpdate struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec HCCFleetUpdateSpec `json:"spec"`

	// +optional
	Status HCCFleetUpdateStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// HCCFleetUpdateList contains a list of HCCFleetUpdate.
type HCCFleetUpdateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []HCCFleetUpdate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HCCFleetUpdate{}, &HCCFleetUpdateList{})
}
