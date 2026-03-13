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

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func int32Ptr(i int32) *int32 { return &i }

func TestCheckDeploymentHealth(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name       string
		objects    []runtime.Object
		wantStatus HealthStatus
		wantErr    bool
	}{
		{
			name:       "no deployments found returns pending",
			objects:    nil,
			wantStatus: HealthStatusPending,
		},
		{
			name: "all replicas ready returns healthy",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(2),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 2,
					},
				},
			},
			wantStatus: HealthStatusHealthy,
		},
		{
			name: "some replicas not ready returns pending",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(3),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 1,
					},
				},
			},
			wantStatus: HealthStatusPending,
		},
		{
			name: "pod in CrashLoopBackOff returns unhealthy",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 0,
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "CrashLoopBackOff",
										Message: "back-off 5m0s restarting failed container",
									},
								},
							},
						},
					},
				},
			},
			wantStatus: HealthStatusUnhealthy,
		},
		{
			name: "pod in ImagePullBackOff returns unhealthy",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 0,
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "ImagePullBackOff",
										Message: "Back-off pulling image",
									},
								},
							},
						},
					},
				},
			},
			wantStatus: HealthStatusUnhealthy,
		},
		{
			name: "pod in ErrImagePull returns unhealthy",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 0,
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason: "ErrImagePull",
									},
								},
							},
						},
					},
				},
			},
			wantStatus: HealthStatusUnhealthy,
		},
		{
			name: "nil replicas treated as 1 replica - ready",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						// Replicas is nil (defaults to 1)
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 1,
					},
				},
			},
			wantStatus: HealthStatusHealthy,
		},
		{
			name: "nil replicas with zero ready returns pending",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deploy",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						// Replicas is nil (defaults to 1)
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 0,
					},
				},
			},
			wantStatus: HealthStatusPending,
		},
		{
			name: "multiple deployments all healthy",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deploy-1",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(2),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 2,
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deploy-2",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 1,
					},
				},
			},
			wantStatus: HealthStatusHealthy,
		},
		{
			name: "multiple deployments with one not ready",
			objects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deploy-1",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(2),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 2,
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deploy-2",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(3),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
					},
					Status: appsv1.DeploymentStatus{
						ReadyReplicas: 1,
					},
				},
			},
			wantStatus: HealthStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.objects != nil {
				objs = tt.objects
			}

			cb := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range objs {
				cb = cb.WithRuntimeObjects(obj)
			}
			c := cb.Build()

			result, err := CheckDeploymentHealth(context.Background(), c, "test-app", "test-ns")
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckDeploymentHealth() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if result.Status != tt.wantStatus {
				t.Errorf("CheckDeploymentHealth() status = %q, want %q (message: %s)", result.Status, tt.wantStatus, result.Message)
			}
		})
	}
}
