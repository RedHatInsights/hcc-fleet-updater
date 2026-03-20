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

	hccv1alpha1 "github.com/RedHatInsights/hcc-operator/api/v1alpha1"
	"github.com/RedHatInsights/hcc-operator/internal/clowdapp"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		wantRepo string
		wantTag  string
	}{
		{
			name:     "standard image with tag",
			image:    "quay.io/cloudservices/advisor-backend:a1b2c3d",
			wantRepo: "quay.io/cloudservices/advisor-backend",
			wantTag:  "a1b2c3d",
		},
		{
			name:     "image without tag",
			image:    "quay.io/cloudservices/advisor-backend",
			wantRepo: "quay.io/cloudservices/advisor-backend",
			wantTag:  "",
		},
		{
			name:     "image with latest tag",
			image:    "quay.io/cloudservices/advisor-backend:latest",
			wantRepo: "quay.io/cloudservices/advisor-backend",
			wantTag:  "latest",
		},
		{
			name:     "image with registry port and tag",
			image:    "registry.example.com:5000/org/image:v1.0",
			wantRepo: "registry.example.com:5000/org/image",
			wantTag:  "v1.0",
		},
		{
			name:     "image with digest",
			image:    "quay.io/cloudservices/advisor-backend@sha256:abc123",
			wantRepo: "quay.io/cloudservices/advisor-backend",
			wantTag:  "@sha256:abc123",
		},
		{
			name:     "simple image name with tag",
			image:    "nginx:1.21",
			wantRepo: "nginx",
			wantTag:  "1.21",
		},
		{
			name:     "template variable style",
			image:    "${IMAGE}:${IMAGE_TAG}",
			wantRepo: "${IMAGE}",
			wantTag:  "${IMAGE_TAG}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag := parseImageRef(tt.image)
			if repo != tt.wantRepo {
				t.Errorf("parseImageRef(%q) repo = %q, want %q", tt.image, repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("parseImageRef(%q) tag = %q, want %q", tt.image, tag, tt.wantTag)
			}
		})
	}
}

func TestBuildImageRef(t *testing.T) {
	tests := []struct {
		repo string
		tag  string
		want string
	}{
		{"quay.io/cloudservices/advisor-backend", "a1b2c3d", "quay.io/cloudservices/advisor-backend:a1b2c3d"},
		{"quay.io/cloudservices/advisor-backend", "", "quay.io/cloudservices/advisor-backend"},
	}

	for _, tt := range tests {
		got := buildImageRef(tt.repo, tt.tag)
		if got != tt.want {
			t.Errorf("buildImageRef(%q, %q) = %q, want %q", tt.repo, tt.tag, got, tt.want)
		}
	}
}

func TestReadClowdAppImages(t *testing.T) {
	app := &clowdapp.ClowdApp{
		Spec: clowdapp.ClowdAppSpec{
			Deployments: []clowdapp.Deployment{
				{
					Name:    "api",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/advisor-backend:a1b2c3d"},
				},
				{
					Name:    "content",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/advisor-content:e4f5g6h"},
				},
			},
			Jobs: []clowdapp.Job{
				{
					Name:    "cleaner",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/advisor-backend:a1b2c3d"},
				},
			},
		},
	}

	images := ReadClowdAppImages(app)
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}

	expected := []hccv1alpha1.ImageTagPair{
		{Image: "quay.io/cloudservices/advisor-backend", Tag: "a1b2c3d"},
		{Image: "quay.io/cloudservices/advisor-content", Tag: "e4f5g6h"},
		{Image: "quay.io/cloudservices/advisor-backend", Tag: "a1b2c3d"},
	}

	for i, img := range images {
		if img.Image != expected[i].Image || img.Tag != expected[i].Tag {
			t.Errorf("image[%d] = {%q, %q}, want {%q, %q}", i, img.Image, img.Tag, expected[i].Image, expected[i].Tag)
		}
	}
}

func TestPatchClowdAppImages_InMemory(t *testing.T) {
	// Test the image matching logic without a real client
	app := &clowdapp.ClowdApp{
		Spec: clowdapp.ClowdAppSpec{
			Deployments: []clowdapp.Deployment{
				{
					Name:    "api",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/advisor-backend:oldtag1"},
				},
				{
					Name:    "worker",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/advisor-content:oldtag2"},
				},
				{
					Name:    "unrelated",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/other-service:unchanged"},
				},
			},
			Jobs: []clowdapp.Job{
				{
					Name:    "migration",
					PodSpec: clowdapp.PodSpec{Image: "quay.io/cloudservices/advisor-backend:oldtag1"},
				},
			},
		},
	}

	desired := []hccv1alpha1.ImageTagPair{
		{Image: "quay.io/cloudservices/advisor-backend", Tag: "newtag1"},
		{Image: "quay.io/cloudservices/advisor-content", Tag: "newtag2"},
	}

	desiredByRepo := make(map[string]string, len(desired))
	for _, d := range desired {
		desiredByRepo[d.Image] = d.Tag
	}

	var patchedCount int
	var previousImages []hccv1alpha1.ImageTagPair //nolint:prealloc // size not known ahead of time

	// Simulate the patching logic (with deduplication, matching PatchClowdAppImages)
	previousByRepo := make(map[string]string)
	for i := range app.Spec.Deployments {
		repo, oldTag := parseImageRef(app.Spec.Deployments[i].PodSpec.Image)
		if newTag, ok := desiredByRepo[repo]; ok {
			if _, seen := previousByRepo[repo]; !seen {
				previousByRepo[repo] = oldTag
			}
			app.Spec.Deployments[i].PodSpec.Image = buildImageRef(repo, newTag)
			patchedCount++
		}
	}
	for i := range app.Spec.Jobs {
		repo, oldTag := parseImageRef(app.Spec.Jobs[i].PodSpec.Image)
		if newTag, ok := desiredByRepo[repo]; ok {
			if _, seen := previousByRepo[repo]; !seen {
				previousByRepo[repo] = oldTag
			}
			app.Spec.Jobs[i].PodSpec.Image = buildImageRef(repo, newTag)
			patchedCount++
		}
	}
	for repo, tag := range previousByRepo {
		previousImages = append(previousImages, hccv1alpha1.ImageTagPair{Image: repo, Tag: tag})
	}

	if patchedCount != 3 {
		t.Errorf("expected 3 patches, got %d", patchedCount)
	}

	// 2 unique repos, not 3 (advisor-backend appears in both deployment and job)
	if len(previousImages) != 2 {
		t.Errorf("expected 2 deduplicated previous images, got %d", len(previousImages))
	}

	// Verify deployments were patched correctly
	if app.Spec.Deployments[0].PodSpec.Image != "quay.io/cloudservices/advisor-backend:newtag1" {
		t.Errorf("deployment[0] image = %q, want %q", app.Spec.Deployments[0].PodSpec.Image, "quay.io/cloudservices/advisor-backend:newtag1")
	}
	if app.Spec.Deployments[1].PodSpec.Image != "quay.io/cloudservices/advisor-content:newtag2" {
		t.Errorf("deployment[1] image = %q, want %q", app.Spec.Deployments[1].PodSpec.Image, "quay.io/cloudservices/advisor-content:newtag2")
	}
	// Verify unrelated deployment was NOT patched
	if app.Spec.Deployments[2].PodSpec.Image != "quay.io/cloudservices/other-service:unchanged" {
		t.Errorf("deployment[2] should not have been patched, got %q", app.Spec.Deployments[2].PodSpec.Image)
	}
	// Verify job was patched
	if app.Spec.Jobs[0].PodSpec.Image != "quay.io/cloudservices/advisor-backend:newtag1" {
		t.Errorf("job[0] image = %q, want %q", app.Spec.Jobs[0].PodSpec.Image, "quay.io/cloudservices/advisor-backend:newtag1")
	}

	// Verify previous images recorded correctly (check by repo since map order is non-deterministic)
	prevMap := make(map[string]string)
	for _, img := range previousImages {
		prevMap[img.Image] = img.Tag
	}
	if prevMap["quay.io/cloudservices/advisor-backend"] != "oldtag1" {
		t.Errorf("previousImages advisor-backend tag = %q, want %q", prevMap["quay.io/cloudservices/advisor-backend"], "oldtag1")
	}
	if prevMap["quay.io/cloudservices/advisor-content"] != "oldtag2" {
		t.Errorf("previousImages advisor-content tag = %q, want %q", prevMap["quay.io/cloudservices/advisor-content"], "oldtag2")
	}
}
