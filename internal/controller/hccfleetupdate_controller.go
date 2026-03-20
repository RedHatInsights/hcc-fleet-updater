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
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	hccv1alpha1 "github.com/RedHatInsights/hcc-operator/api/v1alpha1"
)

const (
	defaultHealthCheckTimeout = 10 * time.Minute
	defaultMaxParallelism     = int32(5)
	requeueInterval           = 15 * time.Second
	pausedRequeueInterval     = 30 * time.Second
)

// HCCFleetUpdateReconciler reconciles a HCCFleetUpdate object.
type HCCFleetUpdateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=hcc.redhat.com,resources=hccfleetupdates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hcc.redhat.com,resources=hccfleetupdates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hcc.redhat.com,resources=hccfleetupdates/finalizers,verbs=update
// +kubebuilder:rbac:groups=cloud.redhat.com,resources=clowdapps,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

//nolint:gocyclo // reconciliation loop is inherently complex
func (r *HCCFleetUpdateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch the HCCFleetUpdate
	fleetUpdate := &hccv1alpha1.HCCFleetUpdate{}
	if err := r.Get(ctx, req.NamespacedName, fleetUpdate); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check for conflicting active fleet updates
	if fleetUpdate.Status.Phase == "" || fleetUpdate.Status.Phase == hccv1alpha1.FleetUpdatePhasePending {
		if conflict, err := r.hasActiveFleetUpdate(ctx, fleetUpdate); err != nil {
			return ctrl.Result{}, err
		} else if conflict != "" {
			log.Info("Another fleet update is active, requeueing", "activeUpdate", conflict)
			r.Recorder.Eventf(fleetUpdate, "Warning", "Blocked", "Another fleet update %q is currently active", conflict)
			return ctrl.Result{RequeueAfter: pausedRequeueInterval}, nil
		}
	}

	// 2. Handle paused state (user-requested or auto-paused by Halt policy)
	paused := fleetUpdate.Spec.Paused || fleetUpdate.Status.AutoPaused
	if paused && fleetUpdate.Status.Phase != hccv1alpha1.FleetUpdatePhasePaused {
		if isTerminal(fleetUpdate.Status.Phase) {
			return ctrl.Result{}, nil
		}
		fleetUpdate.Status.Phase = hccv1alpha1.FleetUpdatePhasePaused
		r.Recorder.Event(fleetUpdate, "Normal", "FleetUpdatePaused", "Fleet update paused by user")
		setCondition(fleetUpdate, "Progressing", metav1.ConditionFalse, "Paused", "Fleet update is paused")
		if err := r.Status().Update(ctx, fleetUpdate); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: pausedRequeueInterval}, nil
	}

	// Resume from pause (user must explicitly set spec.paused=false to clear auto-pause)
	if !fleetUpdate.Spec.Paused && fleetUpdate.Status.Phase == hccv1alpha1.FleetUpdatePhasePaused {
		fleetUpdate.Status.AutoPaused = false
		fleetUpdate.Status.Phase = hccv1alpha1.FleetUpdatePhaseInProgress
		r.Recorder.Event(fleetUpdate, "Normal", "FleetUpdateResumed", "Fleet update resumed")
		setCondition(fleetUpdate, "Progressing", metav1.ConditionTrue, "Resumed", "Fleet update resumed")
		if err := r.Status().Update(ctx, fleetUpdate); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if fleetUpdate.Status.Phase == hccv1alpha1.FleetUpdatePhasePaused {
		return ctrl.Result{RequeueAfter: pausedRequeueInterval}, nil
	}

	// Don't process terminal states
	if isTerminal(fleetUpdate.Status.Phase) {
		return ctrl.Result{}, nil
	}

	// 3. Initialize if new
	if fleetUpdate.Status.Phase == "" {
		r.initializeFleetUpdate(fleetUpdate)
		r.Recorder.Event(fleetUpdate, "Normal", "FleetUpdateStarted", "Fleet update started")
		setCondition(fleetUpdate, "Progressing", metav1.ConditionTrue, "Started", "Fleet update is starting")
		if err := r.Status().Update(ctx, fleetUpdate); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 4. Wave management
	waves := groupByWave(fleetUpdate)
	healthTimeout := r.getHealthTimeout(fleetUpdate)

	for _, wave := range waves {
		waveComplete := true
		for _, idx := range wave {
			appStatus := &fleetUpdate.Status.AppStatuses[idx]
			appSpec := fleetUpdate.Spec.Apps[idx]

			switch appStatus.Phase {
			case hccv1alpha1.AppRolloutPhasePending:
				waveComplete = false
				// Check if we can start this app (parallelism limit)
				if r.countInProgress(fleetUpdate) >= r.getMaxParallelism(fleetUpdate) {
					continue
				}
				// Start the update
				if err := r.startAppUpdate(ctx, fleetUpdate, &appSpec, appStatus); err != nil {
					log.Error(err, "Failed to start app update", "app", appSpec.ClowdAppName)
					appStatus.Phase = hccv1alpha1.AppRolloutPhaseFailed
					appStatus.Message = err.Error()
					now := metav1.Now()
					appStatus.LastTransitionTime = &now
					r.Recorder.Eventf(fleetUpdate, "Warning", "AppUpdateFailed",
						"Failed to update %s: %s", appSpec.ClowdAppName, err.Error())
					if fleetUpdate.Spec.Strategy.FailurePolicy == hccv1alpha1.FailurePolicyHalt {
						fleetUpdate.Status.AutoPaused = true
						r.Recorder.Event(fleetUpdate, "Warning", "FleetUpdateAutopaused",
							"Fleet update auto-paused due to failure (Halt policy)")
					}
				}

			case hccv1alpha1.AppRolloutPhaseInProgress:
				waveComplete = false
				namespace := appSpec.Namespace
				if namespace == "" {
					namespace = fleetUpdate.Namespace
				}
				healthResult, err := CheckDeploymentHealth(ctx, r.Client, appSpec.ClowdAppName, namespace)
				if err != nil {
					log.Error(err, "Health check error", "app", appSpec.ClowdAppName, "namespace", namespace)
					if appStatus.LastTransitionTime != nil &&
						time.Since(appStatus.LastTransitionTime.Time) > healthTimeout {
						now := metav1.Now()
						appStatus.Phase = hccv1alpha1.AppRolloutPhaseFailed
						appStatus.Message = fmt.Sprintf("health check error timeout: %v", err)
						appStatus.LastTransitionTime = &now
						r.Recorder.Eventf(fleetUpdate, "Warning", "AppUpdateFailed",
							"App %s health check timed out with error: %v", appSpec.ClowdAppName, err)
						if fleetUpdate.Spec.Strategy.FailurePolicy == hccv1alpha1.FailurePolicyHalt {
							fleetUpdate.Status.AutoPaused = true
							r.Recorder.Event(fleetUpdate, "Warning", "FleetUpdateAutopaused",
								"Fleet update auto-paused due to failure (Halt policy)")
						}
					}
					continue
				}

				now := metav1.Now()
				switch healthResult.Status {
				case HealthStatusHealthy:
					appStatus.Phase = hccv1alpha1.AppRolloutPhaseSucceeded
					appStatus.Message = healthResult.Message
					appStatus.LastTransitionTime = &now
					r.Recorder.Eventf(fleetUpdate, "Normal", "AppUpdateSucceeded",
						"App %s updated successfully", appSpec.ClowdAppName)

				case HealthStatusUnhealthy:
					// Check if timeout exceeded
					if appStatus.LastTransitionTime != nil &&
						time.Since(appStatus.LastTransitionTime.Time) > healthTimeout {
						appStatus.Phase = hccv1alpha1.AppRolloutPhaseFailed
						appStatus.Message = fmt.Sprintf("health check timeout: %s", healthResult.Message)
						appStatus.LastTransitionTime = &now
						r.Recorder.Eventf(fleetUpdate, "Warning", "AppUpdateFailed",
							"App %s failed health check (timeout): %s", appSpec.ClowdAppName, healthResult.Message)
						if fleetUpdate.Spec.Strategy.FailurePolicy == hccv1alpha1.FailurePolicyHalt {
							fleetUpdate.Status.AutoPaused = true
							r.Recorder.Event(fleetUpdate, "Warning", "FleetUpdateAutopaused",
								"Fleet update auto-paused due to failure (Halt policy)")
						}
					}

				case HealthStatusPending:
					// Check timeout for pending as well
					if appStatus.LastTransitionTime != nil &&
						time.Since(appStatus.LastTransitionTime.Time) > healthTimeout {
						appStatus.Phase = hccv1alpha1.AppRolloutPhaseFailed
						appStatus.Message = fmt.Sprintf("health check timeout: %s", healthResult.Message)
						appStatus.LastTransitionTime = &now
						r.Recorder.Eventf(fleetUpdate, "Warning", "AppUpdateFailed",
							"App %s timed out waiting for health: %s", appSpec.ClowdAppName, healthResult.Message)
						if fleetUpdate.Spec.Strategy.FailurePolicy == hccv1alpha1.FailurePolicyHalt {
							fleetUpdate.Status.AutoPaused = true
							r.Recorder.Event(fleetUpdate, "Warning", "FleetUpdateAutopaused",
								"Fleet update auto-paused due to failure (Halt policy)")
						}
					}
				}
			}
		}

		// Don't advance to next wave until current is complete
		if !waveComplete {
			break
		}
	}

	// 7. Update summary counters
	r.updateSummary(fleetUpdate)

	// 8. Determine overall phase
	r.determineOverallPhase(fleetUpdate)

	// Update status
	if err := r.Status().Update(ctx, fleetUpdate); err != nil {
		return ctrl.Result{}, err
	}

	if isTerminal(fleetUpdate.Status.Phase) {
		r.Recorder.Eventf(fleetUpdate, "Normal", "FleetUpdateCompleted",
			"Fleet update completed with phase %s", fleetUpdate.Status.Phase)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *HCCFleetUpdateReconciler) initializeFleetUpdate(fu *hccv1alpha1.HCCFleetUpdate) {
	if len(fu.Spec.Apps) == 0 {
		fu.Status.Phase = hccv1alpha1.FleetUpdatePhaseFailed
		setCondition(fu, "Degraded", metav1.ConditionTrue, "InvalidSpec",
			"No apps specified in fleet update")
		return
	}

	now := metav1.Now()
	fu.Status.Phase = hccv1alpha1.FleetUpdatePhaseInProgress
	fu.Status.StartTime = &now

	fu.Status.AppStatuses = make([]hccv1alpha1.AppRolloutStatus, len(fu.Spec.Apps))
	for i, app := range fu.Spec.Apps {
		fu.Status.AppStatuses[i] = hccv1alpha1.AppRolloutStatus{
			ClowdAppName: app.ClowdAppName,
			Namespace:    app.Namespace,
			Phase:        hccv1alpha1.AppRolloutPhasePending,
		}
	}
	r.updateSummary(fu)
}

func (r *HCCFleetUpdateReconciler) startAppUpdate(ctx context.Context, fu *hccv1alpha1.HCCFleetUpdate, appSpec *hccv1alpha1.AppImageSpec, appStatus *hccv1alpha1.AppRolloutStatus) error {
	namespace := appSpec.Namespace
	if namespace == "" {
		namespace = fu.Namespace
	}

	app, err := FetchClowdApp(ctx, r.Client, appSpec.ClowdAppName, namespace)
	if err != nil {
		return err
	}

	result, err := PatchClowdAppImages(ctx, r.Client, app, appSpec.Images)
	if err != nil {
		return err
	}

	now := metav1.Now()
	appStatus.Phase = hccv1alpha1.AppRolloutPhaseInProgress
	appStatus.PreviousImages = result.PreviousImages
	appStatus.LastTransitionTime = &now
	appStatus.Message = fmt.Sprintf("patched %d images", result.PatchedCount)

	r.Recorder.Eventf(fu, "Normal", "AppUpdateStarted",
		"Started updating %s (%d images patched)", appSpec.ClowdAppName, result.PatchedCount)

	return nil
}

func (r *HCCFleetUpdateReconciler) countInProgress(fu *hccv1alpha1.HCCFleetUpdate) int32 {
	var count int32
	for _, s := range fu.Status.AppStatuses {
		if s.Phase == hccv1alpha1.AppRolloutPhaseInProgress {
			count++
		}
	}
	return count
}

func (r *HCCFleetUpdateReconciler) getMaxParallelism(fu *hccv1alpha1.HCCFleetUpdate) int32 {
	if fu.Spec.Strategy.MaxParallelism > 0 {
		return fu.Spec.Strategy.MaxParallelism
	}
	return defaultMaxParallelism
}

func (r *HCCFleetUpdateReconciler) getHealthTimeout(fu *hccv1alpha1.HCCFleetUpdate) time.Duration {
	if fu.Spec.Strategy.HealthCheckTimeout != nil {
		return fu.Spec.Strategy.HealthCheckTimeout.Duration
	}
	return defaultHealthCheckTimeout
}

func (r *HCCFleetUpdateReconciler) updateSummary(fu *hccv1alpha1.HCCFleetUpdate) {
	summary := hccv1alpha1.RolloutSummary{
		Total: int32(len(fu.Status.AppStatuses)),
	}
	for _, s := range fu.Status.AppStatuses {
		switch s.Phase {
		case hccv1alpha1.AppRolloutPhasePending:
			summary.Pending++
		case hccv1alpha1.AppRolloutPhaseInProgress:
			summary.InProgress++
		case hccv1alpha1.AppRolloutPhaseSucceeded:
			summary.Succeeded++
		case hccv1alpha1.AppRolloutPhaseFailed:
			summary.Failed++
		case hccv1alpha1.AppRolloutPhaseSkipped:
			summary.Skipped++
		}
	}
	fu.Status.Summary = summary
}

func (r *HCCFleetUpdateReconciler) determineOverallPhase(fu *hccv1alpha1.HCCFleetUpdate) {
	summary := fu.Status.Summary

	// Still work to do
	if summary.InProgress > 0 || summary.Pending > 0 {
		fu.Status.Phase = hccv1alpha1.FleetUpdatePhaseInProgress
		setCondition(fu, "Progressing", metav1.ConditionTrue, "InProgress",
			fmt.Sprintf("%d/%d apps completed", summary.Succeeded+summary.Failed, summary.Total))
		return
	}

	// All done
	now := metav1.Now()
	fu.Status.CompletionTime = &now

	if summary.Failed == 0 {
		fu.Status.Phase = hccv1alpha1.FleetUpdatePhaseCompleted
		setCondition(fu, "Ready", metav1.ConditionTrue, "Completed", "All apps updated successfully")
		setCondition(fu, "Progressing", metav1.ConditionFalse, "Completed", "Fleet update completed")
	} else if summary.Succeeded > 0 {
		fu.Status.Phase = hccv1alpha1.FleetUpdatePhasePartiallyFailed
		setCondition(fu, "Degraded", metav1.ConditionTrue, "PartiallyFailed",
			fmt.Sprintf("%d apps failed, %d succeeded", summary.Failed, summary.Succeeded))
		setCondition(fu, "Progressing", metav1.ConditionFalse, "Completed", "Fleet update completed with failures")
	} else {
		fu.Status.Phase = hccv1alpha1.FleetUpdatePhaseFailed
		setCondition(fu, "Degraded", metav1.ConditionTrue, "Failed", "All apps failed")
		setCondition(fu, "Progressing", metav1.ConditionFalse, "Completed", "Fleet update completed with all failures")
	}
}

// hasActiveFleetUpdate checks if there's already an active (InProgress) fleet update.
func (r *HCCFleetUpdateReconciler) hasActiveFleetUpdate(ctx context.Context, current *hccv1alpha1.HCCFleetUpdate) (string, error) {
	list := &hccv1alpha1.HCCFleetUpdateList{}
	if err := r.List(ctx, list); err != nil {
		return "", err
	}
	for _, fu := range list.Items {
		if fu.Name == current.Name && fu.Namespace == current.Namespace {
			continue
		}
		if fu.Status.Phase == hccv1alpha1.FleetUpdatePhaseInProgress {
			return fu.Name, nil
		}
	}
	return "", nil
}

// groupByWave groups app indices by priority (ascending). Each wave contains
// indices into the spec.Apps / status.AppStatuses arrays.
func groupByWave(fu *hccv1alpha1.HCCFleetUpdate) [][]int {
	type entry struct {
		priority int32
		index    int
	}
	entries := make([]entry, len(fu.Spec.Apps))
	for i, app := range fu.Spec.Apps {
		entries[i] = entry{priority: app.Priority, index: i}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})

	var waves [][]int
	var currentWave []int //nolint:prealloc // size not known ahead of time
	var currentPriority int32 = -1

	for _, e := range entries {
		if e.priority != currentPriority {
			if currentWave != nil {
				waves = append(waves, currentWave)
			}
			currentWave = []int{}
			currentPriority = e.priority
		}
		currentWave = append(currentWave, e.index)
	}
	if currentWave != nil {
		waves = append(waves, currentWave)
	}
	return waves
}

func isTerminal(phase hccv1alpha1.FleetUpdatePhase) bool {
	switch phase {
	case hccv1alpha1.FleetUpdatePhaseCompleted,
		hccv1alpha1.FleetUpdatePhasePartiallyFailed,
		hccv1alpha1.FleetUpdatePhaseFailed:
		return true
	}
	return false
}

func setCondition(fu *hccv1alpha1.HCCFleetUpdate, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&fu.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: fu.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *HCCFleetUpdateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hccv1alpha1.HCCFleetUpdate{}).
		Named("hccfleetupdate").
		Complete(r)
}
