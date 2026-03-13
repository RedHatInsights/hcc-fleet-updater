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
	"strings"

	hccv1alpha1 "github.com/RedHatInsights/hcc-operator/api/v1alpha1"
	"github.com/RedHatInsights/hcc-operator/internal/clowdapp"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchResult contains the result of patching a ClowdApp.
type PatchResult struct {
	PreviousImages []hccv1alpha1.ImageTagPair
	PatchedCount   int
}

// parseImageRef splits an image string like "quay.io/cloudservices/foo:abc123" into repo and tag.
// If no tag is present, tag is empty.
func parseImageRef(image string) (repo, tag string) {
	// Handle images with digest (@sha256:...)
	if idx := strings.LastIndex(image, "@"); idx != -1 {
		return image[:idx], image[idx:]
	}
	// Handle tag separator, but be careful with port numbers in registry URLs.
	// The tag comes after the last colon, but only if the last colon is after the last slash.
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash && lastSlash != -1 {
		return image[:lastColon], image[lastColon+1:]
	}
	if lastColon != -1 && lastSlash == -1 {
		return image[:lastColon], image[lastColon+1:]
	}
	return image, ""
}

// buildImageRef combines a repo and tag into a full image reference.
func buildImageRef(repo, tag string) string {
	if tag == "" {
		return repo
	}
	return repo + ":" + tag
}

// FetchClowdApp retrieves a ClowdApp by name and namespace.
func FetchClowdApp(ctx context.Context, c client.Client, name, namespace string) (*clowdapp.ClowdApp, error) {
	app := &clowdapp.ClowdApp{}
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, app); err != nil {
		return nil, fmt.Errorf("fetching ClowdApp %s/%s: %w", namespace, name, err)
	}
	return app, nil
}

// PatchClowdAppImages patches deployment and job images on a ClowdApp based on the
// desired ImageTagPairs. It returns the previous image values for rollback reference.
func PatchClowdAppImages(ctx context.Context, c client.Client, app *clowdapp.ClowdApp, desired []hccv1alpha1.ImageTagPair) (*PatchResult, error) {
	// Build a lookup map: repo -> desired tag
	desiredByRepo := make(map[string]string, len(desired))
	for _, d := range desired {
		desiredByRepo[d.Image] = d.Tag
	}

	result := &PatchResult{}
	previousByRepo := make(map[string]string)

	// Patch deployment images
	for i := range app.Spec.Deployments {
		repo, oldTag := parseImageRef(app.Spec.Deployments[i].PodSpec.Image)
		if newTag, ok := desiredByRepo[repo]; ok {
			if _, seen := previousByRepo[repo]; !seen {
				previousByRepo[repo] = oldTag
			}
			app.Spec.Deployments[i].PodSpec.Image = buildImageRef(repo, newTag)
			result.PatchedCount++
		}
	}

	// Patch job images
	for i := range app.Spec.Jobs {
		repo, oldTag := parseImageRef(app.Spec.Jobs[i].PodSpec.Image)
		if newTag, ok := desiredByRepo[repo]; ok {
			if _, seen := previousByRepo[repo]; !seen {
				previousByRepo[repo] = oldTag
			}
			app.Spec.Jobs[i].PodSpec.Image = buildImageRef(repo, newTag)
			result.PatchedCount++
		}
	}

	for repo, tag := range previousByRepo {
		result.PreviousImages = append(result.PreviousImages, hccv1alpha1.ImageTagPair{
			Image: repo,
			Tag:   tag,
		})
	}

	if result.PatchedCount == 0 {
		return result, fmt.Errorf("no matching images found in ClowdApp %s/%s", app.Namespace, app.Name)
	}

	if err := c.Update(ctx, app); err != nil {
		return result, fmt.Errorf("updating ClowdApp %s/%s: %w", app.Namespace, app.Name, err)
	}

	return result, nil
}

// ReadClowdAppImages reads the current image:tag pairs from a ClowdApp's deployments and jobs.
func ReadClowdAppImages(app *clowdapp.ClowdApp) []hccv1alpha1.ImageTagPair {
	var images []hccv1alpha1.ImageTagPair
	for _, d := range app.Spec.Deployments {
		repo, tag := parseImageRef(d.PodSpec.Image)
		images = append(images, hccv1alpha1.ImageTagPair{Image: repo, Tag: tag})
	}
	for _, j := range app.Spec.Jobs {
		repo, tag := parseImageRef(j.PodSpec.Image)
		images = append(images, hccv1alpha1.ImageTagPair{Image: repo, Tag: tag})
	}
	return images
}
