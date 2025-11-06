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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	xv1alpha1 "github.com/istio-ecosystem/sail-operator/api/x/v1alpha1"
	"github.com/istio-ecosystem/sail-operator/pkg/constants"
	"github.com/istio-ecosystem/sail-operator/pkg/manifestcustomization"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/postrender"
	"istio.io/istio/operator/pkg/tpath"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	AnnotationPrimaryResource     = "operator-sdk/primary-resource"
	AnnotationPrimaryResourceType = "operator-sdk/primary-resource-type"
)

// NewHelmPostRenderer creates a Helm PostRenderer that adds the following to each rendered manifest:
// - adds the "managed-by: sail-operator" label
// - adds the specified OwnerReference
// - applies user-defined ManifestCustomizations
// It also removes the failurePolicy field from ValidatingWebhookConfigurations on updates, so
// the in-cluster setting stays as-is, to prevent clashing with the istiod validation controller.
func NewHelmPostRenderer(ownerReference *metav1.OwnerReference, ownerNamespace string, isUpdate bool, customizations []xv1alpha1.ManifestCustomization, client client.Client) postrender.PostRenderer {
	return HelmPostRenderer{
		ownerReference: ownerReference,
		ownerNamespace: ownerNamespace,
		isUpdate:       isUpdate,
		customizations: customizations,
		client:         client,
	}
}

type HelmPostRenderer struct {
	ownerReference *metav1.OwnerReference
	ownerNamespace string
	isUpdate       bool
	customizations []xv1alpha1.ManifestCustomization
	client         client.Client
}

var _ postrender.PostRenderer = HelmPostRenderer{}

func (pr HelmPostRenderer) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	modifiedManifests = &bytes.Buffer{}
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

		manifest, err = pr.addOwnerReference(manifest)
		if err != nil {
			return nil, err
		}

		manifest, err = pr.addManagedByLabel(manifest)
		if err != nil {
			return nil, err
		}

		// Strip ValidatingWebhookConfiguration webhooks[].failurePolicy field if we're upgrading,
		// to avoid overwriting the value set in-cluster by the istiod validation controller. On
		// initial install we still want to set the field per the Helm template.
		if pr.isUpdate {
			manifest, err = pr.removeValidatingWebhookFailurePolicy(manifest)
			if err != nil {
				return nil, fmt.Errorf("error removing ValidatingWebhookConfiguration failurePolicy: %v", err)
			}
		}

		// Apply ManifestCustomizations
		manifest, err = pr.applyManifestCustomizations(manifest)
		if err != nil {
			return nil, fmt.Errorf("error applying manifest customizations: %v", err)
		}

		if err := encoder.Encode(manifest); err != nil {
			return nil, err
		}
	}
	return modifiedManifests, nil
}

func (pr HelmPostRenderer) removeValidatingWebhookFailurePolicy(manifest map[string]any) (map[string]any, error) {
	apiVersion, _, _ := unstructured.NestedString(manifest, "apiVersion")
	if apiVersion != admissionregistrationv1.SchemeGroupVersion.String() {
		return manifest, nil
	}
	kind, _, _ := unstructured.NestedString(manifest, "kind")
	if kind != "ValidatingWebhookConfiguration" {
		return manifest, nil
	}

	webhooksAny, found, err := unstructured.NestedFieldNoCopy(manifest, "webhooks")
	if err != nil {
		return nil, err
	}
	if !found {
		return manifest, nil
	}

	webhooks, ok := webhooksAny.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected webhooks to be []interface{}, got %T", webhooksAny)
	}

	for _, webhookAny := range webhooks {
		webhook, ok := webhookAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected webhook to be map[string]interface{}, got %T", webhookAny)
		}

		delete(webhook, "failurePolicy")
	}

	return manifest, nil
}

func (pr HelmPostRenderer) addOwnerReference(manifest map[string]any) (map[string]any, error) {
	if pr.ownerReference == nil {
		return manifest, nil
	}

	objNamespace, objHasNamespace, err := unstructured.NestedFieldNoCopy(manifest, "metadata", "namespace")
	if err != nil {
		return nil, err
	}
	if pr.ownerNamespace == "" || objHasNamespace && objNamespace == pr.ownerNamespace {
		ownerReferences, _, err := unstructured.NestedSlice(manifest, "metadata", "ownerReferences")
		if err != nil {
			return nil, err
		}

		ref, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pr.ownerReference)
		if err != nil {
			return nil, err
		}
		ownerReferences = append(ownerReferences, ref)

		if err := unstructured.SetNestedSlice(manifest, ownerReferences, "metadata", "ownerReferences"); err != nil {
			return nil, err
		}
	} else {
		ownerAPIGroup, _, _ := strings.Cut(pr.ownerReference.APIVersion, "/")
		ownerType := pr.ownerReference.Kind + "." + ownerAPIGroup
		ownerKey := pr.ownerNamespace + "/" + pr.ownerReference.Name
		if err := unstructured.SetNestedField(manifest, ownerType, "metadata", "annotations", AnnotationPrimaryResourceType); err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(manifest, ownerKey, "metadata", "annotations", AnnotationPrimaryResource); err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

func (pr HelmPostRenderer) addManagedByLabel(manifest map[string]any) (map[string]any, error) {
	err := unstructured.SetNestedField(manifest, constants.ManagedByLabelValue, "metadata", "labels", constants.ManagedByLabelKey)
	return manifest, err
}

// applyManifestCustomizations applies user-defined ManifestCustomizations to the manifest
func (pr HelmPostRenderer) applyManifestCustomizations(manifest map[string]any) (map[string]any, error) {
	ctx := context.Background()

	// If no customizations, return as-is
	if len(pr.customizations) == 0 {
		return manifest, nil
	}

	// Convert map[string]any to unstructured.Unstructured for customization processing
	u := &unstructured.Unstructured{Object: manifest}

	// Collect all IGNORE fields for this resource before applying actions
	var ignoredFields []string
	for _, customization := range pr.customizations {
		for _, rule := range customization.Spec.Rules {
			matcher := manifestcustomization.NewResourceMatcher(rule.ApplyTo)
			if !matcher.Matches(u) {
				continue
			}

			// Collect IGNORE operations
			for _, action := range rule.Actions {
				if action.Operation == xv1alpha1.OperationTypeIgnore {
					ignoredFields = append(ignoredFields, action.Field)
				}
			}
		}
	}

	// Apply each customization (which will remove the ignored fields)
	for _, customization := range pr.customizations {
		for _, rule := range customization.Spec.Rules {
			// Check if resource matches any of the applyTo criteria
			matcher := manifestcustomization.NewResourceMatcher(rule.ApplyTo)
			if !matcher.Matches(u) {
				continue
			}

			// Apply all actions from the matching rule
			processor := manifestcustomization.NewActionProcessor(rule.Actions)
			if err := processor.Apply(u); err != nil {
				return nil, fmt.Errorf("failed to apply actions from customization %s/%s: %w",
					customization.Namespace, customization.Name, err)
			}
		}
	}

	// On udpate, restore ignored fields with their values
	// This prevents Helm to see a diff and try to reconcile
	if pr.isUpdate && len(ignoredFields) > 0 {
		if err := pr.restoreIgnoredFields(ctx, u, ignoredFields); err != nil {
			// resource might not exist yet
			logf.FromContext(ctx).V(2).Info("Could not restore ignored fields", "error", err)
		}
	}

	return u.Object, nil
}

// restoreIgnoredFields fetches the resource and copies ignored fields back to manifest
func (pr HelmPostRenderer) restoreIgnoredFields(ctx context.Context, u *unstructured.Unstructured, ignoredFields []string) error {
	if pr.client == nil {
		return fmt.Errorf("client not available")
	}

	resource := &unstructured.Unstructured{}
	resource.SetGroupVersionKind(u.GroupVersionKind())

	key := client.ObjectKey{
		Namespace: u.GetNamespace(),
		Name:      u.GetName(),
	}

	// Fetch the resource
	if err := pr.client.Get(ctx, key, resource); err != nil {
		return fmt.Errorf("failed to fetch resource: %w", err)
	}

	// Copy each ignored field from the resource's state back to the manifest
	for _, fieldPath := range ignoredFields {

		// Get the field value from resource
		value, found, err := pr.getField(ctx, resource, fieldPath)
		if err != nil {
			logf.FromContext(ctx).Info("Error accessing field", "field", fieldPath, "error", err)
			continue
		}

		if !found {
			logf.FromContext(ctx).Info("DEBUG: Field not found in live resource", "field", fieldPath)
			continue
		}

		if value == nil {
			logf.FromContext(ctx).V(2).Info("Field has nil value, skipping", "field", fieldPath)
			continue
		}

		// Set the value in the manifest back
		if err := pr.setField(ctx, u, fieldPath, value); err != nil {
			logf.FromContext(ctx).Error(err, "Failed to set field in manifest", "field", fieldPath)
			continue
		}
	}

	return nil
}

// getField retrieves a field value using tpath (for complex selectors)
// Returns a deep copy of the value
func (pr HelmPostRenderer) getField(ctx context.Context, resource *unstructured.Unstructured, fieldPath string) (interface{}, bool, error) {
	tree := resource.UnstructuredContent()

	// Parse the path properly, respecting selector boundaries
	pathParts := manifestcustomization.ParsePath(fieldPath)

	logf.FromContext(ctx).Info("DEBUG getField: Parsed path",
		"fullPath", fieldPath,
		"pathParts", pathParts)

	// Use tpath to get the field value
	pathContext, found, err := tpath.GetPathContext(tree, pathParts, false)
	if err != nil {
		logf.FromContext(ctx).Info("DEBUG getField: tpath.GetPathContext failed", "pathParts", pathParts, "error", err)
		return nil, false, err
	}

	if !found {
		logf.FromContext(ctx).Info("DEBUG getField: Path not found", "pathParts", pathParts)
		return nil, false, nil
	}

	value := pathContext.Node
	if value == nil {
		logf.FromContext(ctx).Info("DEBUG getField: Field is nil")
		return nil, true, nil
	}

	logf.FromContext(ctx).Info("DEBUG getField: Got field value", "valueType", fmt.Sprintf("%T", value))

	deepCopiedValue, err := deepCopyJSON(value)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deep copy value: %w", err)
	}

	logf.FromContext(ctx).Info("DEBUG getField: Successfully retrieved and deep copied field")
	return deepCopiedValue, true, nil
}

// setField sets a field value using tpath
func (pr HelmPostRenderer) setField(ctx context.Context, u *unstructured.Unstructured, fieldPath string, value interface{}) error {
	tree := u.UnstructuredContent()

	logf.FromContext(ctx).Info("DEBUG setField: Setting field with tpath", "field", fieldPath, "valueType", fmt.Sprintf("%T", value))

	// Deep copy the value before setting to prevent corruption
	deepCopiedValue, err := deepCopyJSON(value)
	if err != nil {
		return fmt.Errorf("failed to deep copy value: %w", err)
	}

	pathParts := manifestcustomization.ParsePath(fieldPath)

	logf.FromContext(ctx).Info("DEBUG setField: Parsed path", "fullPath", fieldPath, "pathParts", pathParts)

	// Use tpath to get/create the path context
	pathContext, found, err := tpath.GetPathContext(tree, pathParts, true)
	if err != nil {
		logf.FromContext(ctx).Error(err, "DEBUG setField: tpath.GetPathContext failed", "pathParts", pathParts)
		return fmt.Errorf("failed to get path context: %w", err)
	}

	logf.FromContext(ctx).Info("DEBUG setField: Got path context", "found", found)

	// Write the value
	if err := tpath.WritePathContext(pathContext, deepCopiedValue); err != nil {
		logf.FromContext(ctx).Error(err, "DEBUG setField: tpath.WritePathContext failed", "pathParts", pathParts)
		return fmt.Errorf("failed to write path context: %w", err)
	}

	u.SetUnstructuredContent(tree)
	logf.FromContext(ctx).Info("DEBUG setField: Successfully set field with tpath", "field", fieldPath)
	return nil
}

// deepCopyJSON performs a deep copy using JSON marshaling/unmarshaling
// This prevents corruption from shared references in complex nested structures
func deepCopyJSON(value interface{}) (interface{}, error) {
	// Marshal to JSON
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value: %w", err)
	}

	// Unmarshal back to get a deep copy
	var result interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return result, nil
}
