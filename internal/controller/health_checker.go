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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HealthStatus represents the health of a ClowdApp's deployments.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "Healthy"
	HealthStatusUnhealthy HealthStatus = "Unhealthy"
	HealthStatusPending   HealthStatus = "Pending"
)

// HealthCheckResult contains the health check outcome.
type HealthCheckResult struct {
	Status  HealthStatus
	Message string
}

// CheckDeploymentHealth checks the health of deployments associated with a ClowdApp.
// It looks for deployments labeled with app=<clowdAppName> in the given namespace.
func CheckDeploymentHealth(ctx context.Context, c client.Client, clowdAppName, namespace string) (*HealthCheckResult, error) {
	deployList := &appsv1.DeploymentList{}
	if err := c.List(ctx, deployList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app": clowdAppName},
	); err != nil {
		return nil, fmt.Errorf("listing deployments for %s/%s: %w", namespace, clowdAppName, err)
	}

	if len(deployList.Items) == 0 {
		return &HealthCheckResult{
			Status:  HealthStatusPending,
			Message: "no deployments found yet",
		}, nil
	}

	for _, deploy := range deployList.Items {
		desired := int32(1)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}
		if deploy.Status.ReadyReplicas < desired {
			// Check pods for error conditions
			msg, err := checkPodsForErrors(ctx, c, deploy.Namespace, deploy.Spec.Selector.MatchLabels)
			if err != nil {
				return nil, err
			}
			if msg != "" {
				return &HealthCheckResult{
					Status:  HealthStatusUnhealthy,
					Message: fmt.Sprintf("deployment %s: %s", deploy.Name, msg),
				}, nil
			}
			return &HealthCheckResult{
				Status:  HealthStatusPending,
				Message: fmt.Sprintf("deployment %s: %d/%d replicas ready", deploy.Name, deploy.Status.ReadyReplicas, desired),
			}, nil
		}
	}

	return &HealthCheckResult{
		Status:  HealthStatusHealthy,
		Message: "all deployments healthy",
	}, nil
}

// checkPodsForErrors checks if any pods matching the given labels have error conditions
// like CrashLoopBackOff, ImagePullBackOff, or ErrImagePull.
func checkPodsForErrors(ctx context.Context, c client.Client, namespace string, labels map[string]string) (string, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	); err != nil {
		return "", fmt.Errorf("listing pods: %w", err)
	}

	errorReasons := map[string]bool{
		"CrashLoopBackOff": true,
		"ImagePullBackOff": true,
		"ErrImagePull":     true,
	}

	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				if errorReasons[cs.State.Waiting.Reason] {
					return fmt.Sprintf("pod %s: %s - %s", pod.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message), nil
				}
			}
		}
	}

	return "", nil
}
