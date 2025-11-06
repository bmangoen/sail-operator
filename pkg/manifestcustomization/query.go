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

	xv1alpha1 "github.com/istio-ecosystem/sail-operator/api/x/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// QueryForTarget queries for ManifestCustomizations that target the specified kind and name
func QueryForTarget(ctx context.Context, c client.Client, targetKind, targetName, namespace string) ([]xv1alpha1.ManifestCustomization, error) {
	// List all ManifestCustomizations in the namespace
	var customizationList xv1alpha1.ManifestCustomizationList
	if err := c.List(ctx, &customizationList, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	// Filter to those that match the target
	var matching []xv1alpha1.ManifestCustomization
	for _, customization := range customizationList.Items {
		for _, targetRef := range customization.Spec.TargetRefs {
			if targetRef.Kind == targetKind && targetRef.Name == targetName {
				matching = append(matching, customization)
				break
			}
		}
	}

	return matching, nil
}

func QueryAll(ctx context.Context, c client.Client, namespace string) ([]xv1alpha1.ManifestCustomization, error) {
	// List all ManifestCustomizations in the namespace
	var customizationList xv1alpha1.ManifestCustomizationList
	if err := c.List(ctx, &customizationList, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	return customizationList.Items, nil
}
