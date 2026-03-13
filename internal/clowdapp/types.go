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

// Package clowdapp provides minimal typed definitions for the ClowdApp CRD
// from github.com/RedHatInsights/clowder. We define only the fields we need
// to read and patch images, avoiding a full dependency on Clowder which has
// incompatible transitive dependencies.
package clowdapp

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: "cloud.redhat.com", Version: "v1alpha1"}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ClowdApp{},
		&ClowdAppList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// ClowdApp is a minimal representation of the ClowdApp CRD.
// Only the fields required for image patching are included.
type ClowdApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClowdAppSpec `json:"spec,omitempty"`
}

// DeepCopyObject implements runtime.Object.
func (in *ClowdApp) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy creates a deep copy of ClowdApp.
func (in *ClowdApp) DeepCopy() *ClowdApp {
	if in == nil {
		return nil
	}
	out := new(ClowdApp)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all properties into another ClowdApp.
func (in *ClowdApp) DeepCopyInto(out *ClowdApp) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// ClowdAppList is a list of ClowdApps.
type ClowdAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClowdApp `json:"items"`
}

// DeepCopyObject implements runtime.Object.
func (in *ClowdAppList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy creates a deep copy of ClowdAppList.
func (in *ClowdAppList) DeepCopy() *ClowdAppList {
	if in == nil {
		return nil
	}
	out := new(ClowdAppList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all properties into another ClowdAppList.
func (in *ClowdAppList) DeepCopyInto(out *ClowdAppList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ClowdApp, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// ClowdAppSpec contains the fields we need from the ClowdApp spec.
type ClowdAppSpec struct {
	Deployments []Deployment `json:"deployments,omitempty"`
	Jobs        []Job        `json:"jobs,omitempty"`
}

// DeepCopyInto copies ClowdAppSpec into another instance.
func (in *ClowdAppSpec) DeepCopyInto(out *ClowdAppSpec) {
	*out = *in
	if in.Deployments != nil {
		out.Deployments = make([]Deployment, len(in.Deployments))
		for i := range in.Deployments {
			in.Deployments[i].DeepCopyInto(&out.Deployments[i])
		}
	}
	if in.Jobs != nil {
		out.Jobs = make([]Job, len(in.Jobs))
		for i := range in.Jobs {
			in.Jobs[i].DeepCopyInto(&out.Jobs[i])
		}
	}
}

// Deployment is a minimal representation of a ClowdApp deployment.
type Deployment struct {
	Name    string  `json:"name"`
	PodSpec PodSpec `json:"podSpec"`
}

// DeepCopyInto copies Deployment into another instance.
func (in *Deployment) DeepCopyInto(out *Deployment) {
	*out = *in
	out.PodSpec = in.PodSpec
}

// Job is a minimal representation of a ClowdApp job.
type Job struct {
	Name    string  `json:"name"`
	PodSpec PodSpec `json:"podSpec"`
}

// DeepCopyInto copies Job into another instance.
func (in *Job) DeepCopyInto(out *Job) {
	*out = *in
	out.PodSpec = in.PodSpec
}

// PodSpec contains the image field we need to patch.
type PodSpec struct {
	Image string `json:"image,omitempty"`
}
