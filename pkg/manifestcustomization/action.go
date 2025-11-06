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

package manifestcustomization

import (
	"context"
	"fmt"

	xv1alpha1 "github.com/istio-ecosystem/sail-operator/api/x/v1alpha1"
	"istio.io/istio/operator/pkg/tpath"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ActionProcessor applies customization actions to resources
type ActionProcessor struct {
	actions []xv1alpha1.CustomizationAction
}

// NewActionProcessor creates a new ActionProcessor with the given actions
func NewActionProcessor(actions []xv1alpha1.CustomizationAction) *ActionProcessor {
	return &ActionProcessor{
		actions: actions,
	}
}

// Apply applies all configured actions to the resource
func (p *ActionProcessor) Apply(resource *unstructured.Unstructured) error {
	for _, action := range p.actions {
		if err := p.applyAction(resource, action); err != nil {
			return fmt.Errorf("failed to apply action %s on field %s: %w", action.Operation, action.Field, err)
		}
	}
	return nil
}

// applyAction applies a single action to the resource
func (p *ActionProcessor) applyAction(resource *unstructured.Unstructured, action xv1alpha1.CustomizationAction) error {
	switch action.Operation {
	case xv1alpha1.OperationTypeIgnore:
		return p.applyIgnore(resource, action.Field)
	default:
		return fmt.Errorf("unsupported operation: %s", action.Operation)
	}
}

// applyIgnore removes the specified field from the resource
func (p *ActionProcessor) applyIgnore(resource *unstructured.Unstructured, fieldPath string) error {
	// Get the resource content tree
	tree := resource.UnstructuredContent()

	// Parse the path properly, respecting selector boundaries
	pathParts := ParsePath(fieldPath)

	// Get the path context for the field to delete
	pathContext, found, err := tpath.GetPathContext(tree, pathParts, false)
	if err != nil {
		// tpath can return errors for invalid path syntax or non-existent paths
		// If the field doesn't exist, there's nothing to delete
		logf.FromContext(context.Background()).V(2).Info("Field path not accessible, skipping ignore", "field", fieldPath, "error", err.Error())
		return nil
	}

	// If the field doesn't exist, there's nothing to delete
	if !found {
		logf.FromContext(context.Background()).V(2).Info("Field not found in manifest, skipping ignore", "field", fieldPath)
		return nil
	}

	// Delete the field by writing nil to it (this is how tpath handles deletions)
	if err := tpath.WritePathContext(pathContext, nil); err != nil {
		return fmt.Errorf("failed to delete field %s: %w", fieldPath, err)
	}

	// Update the resource with the modified tree
	resource.SetUnstructuredContent(tree)
	logf.FromContext(context.Background()).V(2).Info("Successfully removed field from manifest", "field", fieldPath)
	return nil
}
