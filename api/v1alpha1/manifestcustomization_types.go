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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ManifestCustomizationKind = "ManifestCustomization"
)

// ManifestCustomizationOperation defines the type of customization operation to apply
// +kubebuilder:validation:Enum=IGNORE
type ManifestCustomizationOperation string

const (
	// ManifestCustomizationOperationIgnore prevents a field from being updated during reconciliation
	ManifestCustomizationOperationIgnore ManifestCustomizationOperation = "IGNORE"
)

// TargetRef identifies a target resource for customization
type TargetRef struct {
	// Kind specifies the kind of the target resource (Istio, IstioCNI, or ZTunnel).
	// +kubebuilder:validation:Enum=Istio;IstioCNI;ZTunnel
	Kind string `json:"kind"`

	// Name specifies the name of the target resource. If empty, applies to all resources of the specified kind in the namespace.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace specifies the namespace of the target resource. If empty, applies to cluster-scoped resources or all namespaces.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ResourceMatcher defines criteria for matching Kubernetes resources
type ResourceMatcher struct {
	// Group specifies the API group of the resource (e.g., apps, admissionregistration.k8s.io). Empty string matches core group.
	// +optional
	Group string `json:"group,omitempty"`

	// Kind specifies the resource kind (e.g., Deployment, Service).
	Kind string `json:"kind"`

	// Namespace specifies the target namespace. Supports wildcards using * and ?. Empty matches cluster-scoped resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name specifies the resource name. Supports wildcards using * and ?.
	// +optional
	Name string `json:"name,omitempty"`
}

// ManifestCustomizationAction defines an action to apply to a matched resource
type ManifestCustomizationAction struct {
	// Operation specifies the type of customization to apply.
	Operation ManifestCustomizationOperation `json:"operation"`

	// Field specifies the field path to customize using dot notation (e.g., metadata.labels, spec.containers[0].image).
	Field string `json:"field"`

	// Value specifies the value to use for the customization operation (optional, used for future operations).
	// +optional
	Value string `json:"value,omitempty"`
}

// ManifestCustomizationRule defines a customization rule that can be applied to resources
type ManifestCustomizationRule struct {
	// ApplyTo defines a list of resource matchers that determine which generated resources this rule applies to.
	// A resource matches if it satisfies ANY of the applyTo criteria.
	ApplyTo []ResourceMatcher `json:"applyTo"`

	// Actions defines a list of actions to apply to the matched resources. All actions will be applied.
	Actions []ManifestCustomizationAction `json:"actions"`
}

// ManifestCustomizationSpec defines the desired state of ManifestCustomization
type ManifestCustomizationSpec struct {
	// TargetRefs specifies the Istio, IstioCNI, or ZTunnel resources that this customization applies to.
	TargetRefs []TargetRef `json:"targetRefs"`

	// Rules defines the customization rules to apply to matched resources.
	Rules []ManifestCustomizationRule `json:"rules"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=istio-io
// +kubebuilder:printcolumn:name="Targets",type="string",JSONPath=".spec.targetRefs",description="The number of target references this customization applies to."
// +kubebuilder:printcolumn:name="Rules",type="string",JSONPath=".spec.rules",description="The number of rules defined in this customization."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ManifestCustomization allows users to customize Helm-generated Kubernetes manifests
// with fine-grained control over how they are applied to the cluster.
type ManifestCustomization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec ManifestCustomizationSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ManifestCustomizationList contains a list of ManifestCustomization
type ManifestCustomizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManifestCustomization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManifestCustomization{}, &ManifestCustomizationList{})
}
