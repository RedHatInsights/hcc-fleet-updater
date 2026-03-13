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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hccv1alpha1 "github.com/RedHatInsights/hcc-operator/api/v1alpha1"
)

func TestGroupByWave(t *testing.T) {
	fu := &hccv1alpha1.HCCFleetUpdate{
		Spec: hccv1alpha1.HCCFleetUpdateSpec{
			Apps: []hccv1alpha1.AppImageSpec{
				{ClowdAppName: "app-c", Priority: 10},
				{ClowdAppName: "app-a", Priority: 1},
				{ClowdAppName: "app-b", Priority: 5},
				{ClowdAppName: "app-d", Priority: 1},
			},
		},
	}

	waves := groupByWave(fu)

	if len(waves) != 3 {
		t.Fatalf("expected 3 waves, got %d", len(waves))
	}

	// Wave 0: priority 1 (app-a at index 1, app-d at index 3)
	if len(waves[0]) != 2 {
		t.Errorf("wave 0 should have 2 apps, got %d", len(waves[0]))
	}
	if fu.Spec.Apps[waves[0][0]].Priority != 1 || fu.Spec.Apps[waves[0][1]].Priority != 1 {
		t.Errorf("wave 0 apps should have priority 1")
	}

	// Wave 1: priority 5 (app-b at index 2)
	if len(waves[1]) != 1 {
		t.Errorf("wave 1 should have 1 app, got %d", len(waves[1]))
	}
	if fu.Spec.Apps[waves[1][0]].ClowdAppName != "app-b" {
		t.Errorf("wave 1 should contain app-b, got %s", fu.Spec.Apps[waves[1][0]].ClowdAppName)
	}

	// Wave 2: priority 10 (app-c at index 0)
	if len(waves[2]) != 1 {
		t.Errorf("wave 2 should have 1 app, got %d", len(waves[2]))
	}
	if fu.Spec.Apps[waves[2][0]].ClowdAppName != "app-c" {
		t.Errorf("wave 2 should contain app-c, got %s", fu.Spec.Apps[waves[2][0]].ClowdAppName)
	}
}

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		phase hccv1alpha1.FleetUpdatePhase
		want  bool
	}{
		{hccv1alpha1.FleetUpdatePhaseCompleted, true},
		{hccv1alpha1.FleetUpdatePhasePartiallyFailed, true},
		{hccv1alpha1.FleetUpdatePhaseFailed, true},
		{hccv1alpha1.FleetUpdatePhaseInProgress, false},
		{hccv1alpha1.FleetUpdatePhasePending, false},
		{hccv1alpha1.FleetUpdatePhasePaused, false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isTerminal(tt.phase); got != tt.want {
			t.Errorf("isTerminal(%q) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}

func TestUpdateSummary(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Status: hccv1alpha1.HCCFleetUpdateStatus{
			AppStatuses: []hccv1alpha1.AppRolloutStatus{
				{Phase: hccv1alpha1.AppRolloutPhasePending},
				{Phase: hccv1alpha1.AppRolloutPhaseInProgress},
				{Phase: hccv1alpha1.AppRolloutPhaseSucceeded},
				{Phase: hccv1alpha1.AppRolloutPhaseSucceeded},
				{Phase: hccv1alpha1.AppRolloutPhaseFailed},
				{Phase: hccv1alpha1.AppRolloutPhaseSkipped},
			},
		},
	}

	r.updateSummary(fu)

	if fu.Status.Summary.Total != 6 {
		t.Errorf("Total = %d, want 6", fu.Status.Summary.Total)
	}
	if fu.Status.Summary.Pending != 1 {
		t.Errorf("Pending = %d, want 1", fu.Status.Summary.Pending)
	}
	if fu.Status.Summary.InProgress != 1 {
		t.Errorf("InProgress = %d, want 1", fu.Status.Summary.InProgress)
	}
	if fu.Status.Summary.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", fu.Status.Summary.Succeeded)
	}
	if fu.Status.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", fu.Status.Summary.Failed)
	}
	if fu.Status.Summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", fu.Status.Summary.Skipped)
	}
}

func TestDetermineOverallPhase_AllSucceeded(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Status: hccv1alpha1.HCCFleetUpdateStatus{
			Phase: hccv1alpha1.FleetUpdatePhaseInProgress,
			Summary: hccv1alpha1.RolloutSummary{
				Total: 3, Succeeded: 3,
			},
		},
	}

	r.determineOverallPhase(fu)

	if fu.Status.Phase != hccv1alpha1.FleetUpdatePhaseCompleted {
		t.Errorf("phase = %q, want %q", fu.Status.Phase, hccv1alpha1.FleetUpdatePhaseCompleted)
	}
	if fu.Status.CompletionTime == nil {
		t.Error("completionTime should be set")
	}
}

func TestDetermineOverallPhase_PartiallyFailed(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Status: hccv1alpha1.HCCFleetUpdateStatus{
			Phase: hccv1alpha1.FleetUpdatePhaseInProgress,
			Summary: hccv1alpha1.RolloutSummary{
				Total: 3, Succeeded: 2, Failed: 1,
			},
		},
	}

	r.determineOverallPhase(fu)

	if fu.Status.Phase != hccv1alpha1.FleetUpdatePhasePartiallyFailed {
		t.Errorf("phase = %q, want %q", fu.Status.Phase, hccv1alpha1.FleetUpdatePhasePartiallyFailed)
	}
}

func TestDetermineOverallPhase_AllFailed(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Status: hccv1alpha1.HCCFleetUpdateStatus{
			Phase: hccv1alpha1.FleetUpdatePhaseInProgress,
			Summary: hccv1alpha1.RolloutSummary{
				Total: 3, Failed: 3,
			},
		},
	}

	r.determineOverallPhase(fu)

	if fu.Status.Phase != hccv1alpha1.FleetUpdatePhaseFailed {
		t.Errorf("phase = %q, want %q", fu.Status.Phase, hccv1alpha1.FleetUpdatePhaseFailed)
	}
}

func TestDetermineOverallPhase_StillInProgress(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Status: hccv1alpha1.HCCFleetUpdateStatus{
			Phase: hccv1alpha1.FleetUpdatePhaseInProgress,
			Summary: hccv1alpha1.RolloutSummary{
				Total: 3, Succeeded: 1, InProgress: 1, Pending: 1,
			},
		},
	}

	r.determineOverallPhase(fu)

	if fu.Status.Phase != hccv1alpha1.FleetUpdatePhaseInProgress {
		t.Errorf("phase = %q, want %q", fu.Status.Phase, hccv1alpha1.FleetUpdatePhaseInProgress)
	}
	if fu.Status.CompletionTime != nil {
		t.Error("completionTime should not be set while in progress")
	}
}

func TestInitializeFleetUpdate(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Spec: hccv1alpha1.HCCFleetUpdateSpec{
			Apps: []hccv1alpha1.AppImageSpec{
				{ClowdAppName: "app-a", Namespace: "ns-a"},
				{ClowdAppName: "app-b", Namespace: "ns-b"},
			},
		},
	}

	r.initializeFleetUpdate(fu)

	if fu.Status.Phase != hccv1alpha1.FleetUpdatePhaseInProgress {
		t.Errorf("phase = %q, want %q", fu.Status.Phase, hccv1alpha1.FleetUpdatePhaseInProgress)
	}
	if fu.Status.StartTime == nil {
		t.Error("startTime should be set")
	}
	if len(fu.Status.AppStatuses) != 2 {
		t.Fatalf("expected 2 app statuses, got %d", len(fu.Status.AppStatuses))
	}
	for _, s := range fu.Status.AppStatuses {
		if s.Phase != hccv1alpha1.AppRolloutPhasePending {
			t.Errorf("initial app phase = %q, want %q", s.Phase, hccv1alpha1.AppRolloutPhasePending)
		}
	}
	if fu.Status.Summary.Total != 2 {
		t.Errorf("summary.Total = %d, want 2", fu.Status.Summary.Total)
	}
	if fu.Status.Summary.Pending != 2 {
		t.Errorf("summary.Pending = %d, want 2", fu.Status.Summary.Pending)
	}
}

func TestInitializeFleetUpdate_EmptyApps(t *testing.T) {
	r := &HCCFleetUpdateReconciler{}
	fu := &hccv1alpha1.HCCFleetUpdate{
		Spec: hccv1alpha1.HCCFleetUpdateSpec{
			Apps: []hccv1alpha1.AppImageSpec{},
		},
	}

	r.initializeFleetUpdate(fu)

	if fu.Status.Phase != hccv1alpha1.FleetUpdatePhaseFailed {
		t.Errorf("phase = %q, want %q for empty apps", fu.Status.Phase, hccv1alpha1.FleetUpdatePhaseFailed)
	}
	if len(fu.Status.Conditions) == 0 {
		t.Error("expected a Degraded condition for empty apps")
	}
}

func TestSetCondition(t *testing.T) {
	fu := &hccv1alpha1.HCCFleetUpdate{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
	}

	setCondition(fu, "Ready", metav1.ConditionTrue, "TestReason", "test message")

	if len(fu.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(fu.Status.Conditions))
	}
	if fu.Status.Conditions[0].Type != "Ready" {
		t.Errorf("condition type = %q, want %q", fu.Status.Conditions[0].Type, "Ready")
	}
	if fu.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("condition status = %q, want %q", fu.Status.Conditions[0].Status, metav1.ConditionTrue)
	}

	// Update existing condition
	setCondition(fu, "Ready", metav1.ConditionFalse, "Updated", "updated message")
	if len(fu.Status.Conditions) != 1 {
		t.Errorf("should still have 1 condition after update, got %d", len(fu.Status.Conditions))
	}
	if fu.Status.Conditions[0].Status != metav1.ConditionFalse {
		t.Errorf("updated condition status = %q, want %q", fu.Status.Conditions[0].Status, metav1.ConditionFalse)
	}
}
