// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/istio-ecosystem/sail-operator/api/v1alpha1"
	"github.com/istio-ecosystem/sail-operator/pkg/manifestcustomization"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/postrender"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManifestCustomizationPostRenderer extends the HelmPostRenderer to include manifest customization
type ManifestCustomizationPostRenderer struct {
	HelmPostRenderer
	client    client.Client
	targetRef v1alpha1.TargetRef
	ctx       context.Context
	customizer *manifestcustomization.Customizer
}

// NewManifestCustomizationPostRenderer creates a PostRenderer that applies manifest customizations
func NewManifestCustomizationPostRenderer(
	client client.Client,
	ctx context.Context,
	targetRef v1alpha1.TargetRef,
	helmPostRenderer HelmPostRenderer,
) postrender.PostRenderer {
	return &ManifestCustomizationPostRenderer{
		HelmPostRenderer: helmPostRenderer,
		client:           client,
		targetRef:        targetRef,
		ctx:              ctx,
		customizer:       manifestcustomization.NewCustomizer(client),
	}
}

var _ postrender.PostRenderer = &ManifestCustomizationPostRenderer{}

func (pr *ManifestCustomizationPostRenderer) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	// First, apply the standard Helm post-rendering
	standardProcessed, err := pr.HelmPostRenderer.Run(renderedManifests)
	if err != nil {
		return nil, fmt.Errorf("failed to apply standard helm post-rendering: %w", err)
	}

	// Now apply manifest customizations
	return pr.applyManifestCustomizations(standardProcessed)
}

func (pr *ManifestCustomizationPostRenderer) applyManifestCustomizations(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	modifiedManifests := &bytes.Buffer{}
	encoder := yaml.NewEncoder(modifiedManifests)
	encoder.SetIndent(2)
	decoder := yaml.NewDecoder(renderedManifests)

	for {
		manifest := map[string]any{}

		if err := decoder.Decode(&manifest); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if manifest == nil {
			continue
		}

		// Convert to unstructured for easier processing
		obj := &unstructured.Unstructured{Object: manifest}

		// Apply manifest customizations (remove ignored fields)
		if err := pr.customizer.RemoveIgnoredFields(pr.ctx, pr.targetRef, obj); err != nil {
			return nil, fmt.Errorf("failed to apply manifest customizations: %w", err)
		}

		// Encode the modified manifest
		if err := encoder.Encode(obj.Object); err != nil {
			return nil, err
		}
	}

	return modifiedManifests, nil
}