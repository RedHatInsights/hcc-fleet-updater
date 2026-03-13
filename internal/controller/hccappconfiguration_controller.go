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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	hccv1alpha1 "github.com/RedHatInsights/hcc-operator/api/v1alpha1"
)

const appConfigRequeueInterval = 60 * time.Second

// HCCAppConfigurationReconciler reconciles a HCCAppConfiguration object.
type HCCAppConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=hcc.redhat.com,resources=hccappconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hcc.redhat.com,resources=hccappconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hcc.redhat.com,resources=hccappconfigurations/finalizers,verbs=update

func (r *HCCAppConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the HCCAppConfiguration
	appConfig := &hccv1alpha1.HCCAppConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, appConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if monitoring is enabled
	if appConfig.Spec.Enabled != nil && !*appConfig.Spec.Enabled {
		return ctrl.Result{RequeueAfter: appConfigRequeueInterval}, nil
	}

	namespace := appConfig.Spec.Namespace
	if namespace == "" {
		namespace = appConfig.Namespace
	}

	// Fetch the ClowdApp
	clowdApp, err := FetchClowdApp(ctx, r.Client, appConfig.Spec.ClowdAppName, namespace)
	if err != nil {
		log.Error(err, "Failed to fetch ClowdApp", "clowdApp", appConfig.Spec.ClowdAppName)
		healthy := false
		appConfig.Status.Healthy = &healthy
		now := metav1.Now()
		appConfig.Status.LastUpdated = &now
		if statusErr := r.Status().Update(ctx, appConfig); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: appConfigRequeueInterval}, nil
	}

	// Read current images
	appConfig.Status.CurrentImages = ReadClowdAppImages(clowdApp)

	// Check deployment health
	healthResult, err := CheckDeploymentHealth(ctx, r.Client, appConfig.Spec.ClowdAppName, namespace)
	if err != nil {
		log.Error(err, "Health check failed", "clowdApp", appConfig.Spec.ClowdAppName)
	} else {
		healthy := healthResult.Status == HealthStatusHealthy
		appConfig.Status.Healthy = &healthy
	}

	now := metav1.Now()
	appConfig.Status.LastUpdated = &now

	if err := r.Status().Update(ctx, appConfig); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: appConfigRequeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HCCAppConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hccv1alpha1.HCCAppConfiguration{}).
		Named("hccappconfiguration").
		Complete(r)
}
