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
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/istio-ecosystem/sail-operator/api/v1alpha1"
)

// Customizer handles manifest customization operations
type Customizer struct {
	client client.Client
}

// NewCustomizer creates a new Customizer instance
func NewCustomizer(client client.Client) *Customizer {
	return &Customizer{
		client: client,
	}
}

type IgnoreFieldSpec struct {
	GroupVersionKind schema.GroupVersionKind
	Namespace        string
	Name             string
	FieldPath        string
}

func (c *Customizer) GetIgnoreFields(ctx context.Context, targetRef v1alpha1.TargetRef) ([]IgnoreFieldSpec, error) {
	var ignoreFields []IgnoreFieldSpec
	customizations := &v1alpha1.ManifestCustomizationList{}
	if err := c.client.List(ctx, customizations); err != nil {
		return nil, fmt.Errorf("failed to list ManifestCustomization resources: %w", err)
	}

	for _, customization := range customizations.Items {
		if c.matchesTargetRef(customization.Spec.TargetRefs, targetRef) {
			ignoreSpecs := c.extractIgnoreFields(customization.Spec.Rules)
			ignoreFields = append(ignoreFields, ignoreSpecs...)
		}
	}

	return ignoreFields, nil
}

func (c *Customizer) ShouldIgnoreField(ctx context.Context, targetRef v1alpha1.TargetRef, obj *unstructured.Unstructured, fieldPath string) (bool, error) {
	ignoreFields, err := c.GetIgnoreFields(ctx, targetRef)
	if err != nil {
		return false, err
	}

	gvk := obj.GroupVersionKind()
	namespace := obj.GetNamespace()
	name := obj.GetName()

	for _, ignoreField := range ignoreFields {
		if c.matchesResource(ignoreField, gvk, namespace, name) && c.matchesFieldPath(ignoreField.FieldPath, fieldPath) {
			return true, nil
		}
	}

	return false, nil
}

func (c *Customizer) RemoveIgnoredFields(ctx context.Context, targetRef v1alpha1.TargetRef, obj *unstructured.Unstructured) error {
	ignoreFields, err := c.GetIgnoreFields(ctx, targetRef)
	if err != nil {
		return err
	}

	gvk := obj.GroupVersionKind()
	namespace := obj.GetNamespace()
	name := obj.GetName()

	for _, ignoreField := range ignoreFields {
		if c.matchesResource(ignoreField, gvk, namespace, name) {
			if err := c.removeFieldFromObject(obj, ignoreField.FieldPath); err != nil {
				return fmt.Errorf("failed to remove ignored field %s: %w", ignoreField.FieldPath, err)
			}
		}
	}

	return nil
}

func (c *Customizer) matchesTargetRef(targetRefs []v1alpha1.TargetRef, target v1alpha1.TargetRef) bool {
	for _, ref := range targetRefs {
		if ref.Kind != target.Kind {
			continue
		}

		if ref.Name != "" && ref.Name != target.Name {
			continue
		}

		if ref.Namespace != "" && ref.Namespace != target.Namespace {
			continue
		}

		return true
	}
	return false
}

func (c *Customizer) extractIgnoreFields(rules []v1alpha1.ManifestCustomizationRule) []IgnoreFieldSpec {
	var ignoreFields []IgnoreFieldSpec

	for _, rule := range rules {
		for _, action := range rule.Actions {
			if action.Operation == v1alpha1.ManifestCustomizationOperationIgnore {
				for _, matcher := range rule.ApplyTo {
					gvk := schema.GroupVersionKind{
						Group: matcher.Group,
						Kind:  matcher.Kind,
					}
					// TODO: make this configurable
					if gvk.Group == "" {
						gvk.Version = "v1"
					} else {
						gvk.Version = "v1"
					}

					ignoreFields = append(ignoreFields, IgnoreFieldSpec{
						GroupVersionKind: gvk,
						Namespace:        matcher.Namespace,
						Name:             matcher.Name,
						FieldPath:        action.Field,
					})
				}
			}
		}
	}

	return ignoreFields
}

func (c *Customizer) matchesResource(ignoreField IgnoreFieldSpec, gvk schema.GroupVersionKind, namespace, name string) bool {
	// Match GroupVersionKind (ignoring version for now, focusing on Group and Kind)
	if ignoreField.GroupVersionKind.Group != gvk.Group || ignoreField.GroupVersionKind.Kind != gvk.Kind {
		return false
	}

	if ignoreField.Namespace != "" && !c.matchesWithWildcard(ignoreField.Namespace, namespace) {
		return false
	}

	if ignoreField.Name != "" && !c.matchesWithWildcard(ignoreField.Name, name) {
		return false
	}

	return true
}

func (c *Customizer) matchesWithWildcard(pattern, str string) bool {
	matched, _ := filepath.Match(pattern, str)
	return matched
}

func (c *Customizer) matchesFieldPath(ignorePath, fieldPath string) bool {
	return ignorePath == fieldPath
}

// TODO: Use a proper matcher according to the chosen solution from the design document
func (c *Customizer) removeFieldFromObject(obj *unstructured.Unstructured, fieldPath string) error {
	pathParts := strings.Split(fieldPath, ".")
	if len(pathParts) == 0 {
		return fmt.Errorf("empty field path")
	}

	content := obj.Object

	for i := 0; i < len(pathParts)-1; i++ {
		part := pathParts[i]

		next, exists := content[part]
		if !exists {
			return nil
		}

		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("field %s is not a map in path %s", part, fieldPath)
		}

		content = nextMap
	}

	// Remove the final field
	finalField := pathParts[len(pathParts)-1]
	delete(content, finalField)

	return nil
}
