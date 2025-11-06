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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ManifestCustomizationKind = "ManifestCustomization"
)

// ManifestCustomizationSpec defines the desired state of ManifestCustomization
type ManifestCustomizationSpec struct {
	// TargetRefs specifies the Istio, IstioCNI, or ZTunnel resources to which this customization applies.
	// +kubebuilder:validation:MinItems=1
	TargetRefs []TargetRef `json:"targetRefs"`

	// Rules defines a list of customization rules to apply to the matched resources.
	// +kubebuilder:validation:MinItems=1
	Rules []CustomizationRule `json:"rules"`
}

// TargetRef identifies a target resource to which customizations should be applied
type TargetRef struct {
	// Kind of the target resource. Must be one of: Istio, IstioCNI, ZTunnel
	// +kubebuilder:validation:Enum=Istio;IstioCNI;ZTunnel
	Kind string `json:"kind"`

	// Name of the target resource
	Name string `json:"name"`
}

// CustomizationRule defines a rule for customizing matched resources
type CustomizationRule struct {
	// ApplyTo specifies which resources this rule applies to.
	// A resource matches if it matches ANY of the applyTo criteria.
	// +kubebuilder:validation:MinItems=1
	ApplyTo []ResourceMatcher `json:"applyTo"`

	// Actions specifies the list of actions to apply to matched resources.
	// All actions in the list will be applied.
	// +kubebuilder:validation:MinItems=1
	Actions []CustomizationAction `json:"actions"`
}

// ResourceMatcher defines criteria for matching Kubernetes resources
type ResourceMatcher struct {
	// Group is the API group of the resource (e.g., "apps", "v1", "admissionregistration.k8s.io").
	// Empty string matches the core API group.
	// +optional
	Group string `json:"group,omitempty"`

	// Kind is the kind of the resource (e.g., "Deployment", "ConfigMap", "MutatingWebhookConfiguration").
	Kind string `json:"kind"`

	// Namespace is the namespace of the resource. Supports wildcards.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name is the name of the resource. Supports wildcards.
	// +optional
	Name string `json:"name,omitempty"`
}

// CustomizationAction defines an action to apply to a matched resource
type CustomizationAction struct {
	// Operation specifies the type of operation to perform.
	// Currently only IGNORE is supported.
	// +kubebuilder:validation:Enum=IGNORE
	Operation OperationType `json:"operation"`

	// Field specifies the field path to which the operation applies.
	// Uses the Istio operator tpath syntax for field path resolution.
	// Examples:
	//   - "metadata.labels.foo"
	//   - "spec.replicas"
	//   - "webhooks[name:my-webhook].namespaceSelector"
	Field string `json:"field"`
}

// OperationType specifies the type of operation to perform on a field
type OperationType string

const (
	// OperationTypeIgnore removes the field from the manifest, preventing reconciliation
	OperationTypeIgnore OperationType = "IGNORE"
)

// ManifestCustomizationStatus defines the observed state of ManifestCustomization
type ManifestCustomizationStatus struct {
	// ObservedGeneration is the most recent generation observed for this
	// ManifestCustomization object. It corresponds to the object's generation, which is
	// updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of the object's current state.
	// +optional
	Conditions []ManifestCustomizationCondition `json:"conditions,omitempty"`

	// State reports the current state of the object.
	// +optional
	State ManifestCustomizationConditionReason `json:"state,omitempty"`

	// TargetRefs contains the status for each target reference
	// +optional
	TargetRefStatuses []TargetRefStatus `json:"targetRefStatuses,omitempty"`
}

// TargetRefStatus represents the status of a target reference
type TargetRefStatus struct {
	// Kind is the kind of the target resource
	Kind string `json:"kind"`

	// Name is the name of the target resource
	Name string `json:"name"`

	// Applied indicates whether the customization has been successfully applied to this target
	Applied bool `json:"applied"`

	// Message provides additional information about the status
	// +optional
	Message string `json:"message,omitempty"`
}

// GetCondition returns the condition of the specified type
func (s *ManifestCustomizationStatus) GetCondition(conditionType ManifestCustomizationConditionType) ManifestCustomizationCondition {
	if s != nil {
		for i := range s.Conditions {
			if s.Conditions[i].Type == conditionType {
				return s.Conditions[i]
			}
		}
	}
	return ManifestCustomizationCondition{Type: conditionType, Status: metav1.ConditionUnknown}
}

// SetCondition sets a specific condition in the list of conditions
func (s *ManifestCustomizationStatus) SetCondition(condition ManifestCustomizationCondition) {
	var now time.Time
	if testTime == nil {
		now = time.Now()
	} else {
		now = *testTime
	}

	// The lastTransitionTime only gets serialized out to the second.  This can
	// break update skipping, as the time in the resource returned from the client
	// may not match the time in our cached status during a reconcile.  We truncate
	// here to save any problems down the line.
	lastTransitionTime := metav1.NewTime(now.Truncate(time.Second))

	for i, prevCondition := range s.Conditions {
		if prevCondition.Type == condition.Type {
			if prevCondition.Status != condition.Status {
				condition.LastTransitionTime = lastTransitionTime
			} else {
				condition.LastTransitionTime = prevCondition.LastTransitionTime
			}
			s.Conditions[i] = condition
			return
		}
	}

	// If the condition does not exist, initialize the lastTransitionTime
	condition.LastTransitionTime = lastTransitionTime
	s.Conditions = append(s.Conditions, condition)
}

// ManifestCustomizationCondition represents a specific observation of the ManifestCustomization object's state.
type ManifestCustomizationCondition struct {
	// Type is the type of this condition.
	// +optional
	Type ManifestCustomizationConditionType `json:"type,omitempty"`

	// Status is the status of this condition. Can be True, False or Unknown.
	// +optional
	Status metav1.ConditionStatus `json:"status,omitempty"`

	// Reason is a unique, single-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason ManifestCustomizationConditionReason `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about the last transition.
	// +optional
	Message string `json:"message,omitempty"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// ManifestCustomizationConditionType represents the type of the condition.
type ManifestCustomizationConditionType string

// ManifestCustomizationConditionReason represents a short message indicating how the condition came
// to be in its present state.
type ManifestCustomizationConditionReason string

const (
	// ManifestCustomizationConditionReconciled signifies whether the controller has
	// successfully reconciled the resources defined through the CR.
	ManifestCustomizationConditionReconciled ManifestCustomizationConditionType = "Reconciled"

	// ManifestCustomizationReasonReconcileError indicates that the reconciliation of the resource has failed, but will be retried.
	ManifestCustomizationReasonReconcileError ManifestCustomizationConditionReason = "ReconcileError"
)

const (
	// ManifestCustomizationReasonHealthy indicates that the customization is fully reconciled and applied.
	ManifestCustomizationReasonHealthy ManifestCustomizationConditionReason = "Healthy"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories=istio-io
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="The current state of this object."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="The age of the object"

// ManifestCustomization is the Schema for the manifestcustomizations API
type ManifestCustomization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec ManifestCustomizationSpec `json:"spec,omitempty"`

	// +optional
	Status ManifestCustomizationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ManifestCustomizationList contains a list of ManifestCustomization
type ManifestCustomizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManifestCustomization `json:"items"`
}

var testTime *time.Time // Field for mocking time in tests

func init() {
	SchemeBuilder.Register(&ManifestCustomization{}, &ManifestCustomizationList{})
}
