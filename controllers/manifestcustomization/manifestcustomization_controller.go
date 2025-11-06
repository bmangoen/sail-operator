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
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	xv1alpha1 "github.com/istio-ecosystem/sail-operator/api/x/v1alpha1"
	"github.com/istio-ecosystem/sail-operator/pkg/constants"
	"github.com/istio-ecosystem/sail-operator/pkg/enqueuelogger"
	"github.com/istio-ecosystem/sail-operator/pkg/errlist"
	"github.com/istio-ecosystem/sail-operator/pkg/kube"
	"github.com/istio-ecosystem/sail-operator/pkg/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a ManifestCustomization object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func NewReconciler(client client.Client, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		Client: client,
		Scheme: scheme,
	}
}

// +kubebuilder:rbac:groups=x.sailoperator.io,resources=manifestcustomizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=x.sailoperator.io,resources=manifestcustomizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=x.sailoperator.io,resources=manifestcustomizations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, mc *xv1alpha1.ManifestCustomization) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	reconcileErr := r.doReconcile(ctx, mc)

	log.Info("Reconciliation done. Updating status.")
	statusErr := r.updateStatus(ctx, mc, reconcileErr)

	return ctrl.Result{}, errors.Join(reconcileErr, statusErr)
}

func (r *Reconciler) Finalize(ctx context.Context, mc *xv1alpha1.ManifestCustomization) error {
	// TODO: Is this needed?
	return nil
}

func (r *Reconciler) doReconcile(ctx context.Context, mc *xv1alpha1.ManifestCustomization) error {
	log := logf.FromContext(ctx)

	if err := r.validate(ctx, mc); err != nil {
		return err
	}

	// Verify that the target resources exist
	targetStatuses := make([]xv1alpha1.TargetRefStatus, 0, len(mc.Spec.TargetRefs))
	for _, targetRef := range mc.Spec.TargetRefs {
		status := r.verifyTargetRef(ctx, mc.Namespace, targetRef)
		targetStatuses = append(targetStatuses, status)
	}

	// Log the validation results
	for _, status := range targetStatuses {
		if !status.Applied {
			log.Info("Target reference not found or not valid", "kind", status.Kind, "name", status.Name, "message", status.Message)
		} else {
			log.Info("Target reference validated successfully", "kind", status.Kind, "name", status.Name)
		}
	}

	return nil
}

func (r *Reconciler) validate(ctx context.Context, mc *xv1alpha1.ManifestCustomization) error {
	if len(mc.Spec.TargetRefs) == 0 {
		return reconciler.NewValidationError("spec.targetRefs cannot be empty")
	}
	if len(mc.Spec.Rules) == 0 {
		return reconciler.NewValidationError("spec.rules cannot be empty")
	}

	// Validate each rule
	for i, rule := range mc.Spec.Rules {
		if len(rule.ApplyTo) == 0 {
			return reconciler.NewValidationError(fmt.Sprintf("spec.rules[%d].applyTo cannot be empty", i))
		}
		if len(rule.Actions) == 0 {
			return reconciler.NewValidationError(fmt.Sprintf("spec.rules[%d].actions cannot be empty", i))
		}

		// Validate each action
		for j, action := range rule.Actions {
			if action.Operation == "" {
				return reconciler.NewValidationError(fmt.Sprintf("spec.rules[%d].actions[%d].operation cannot be empty", i, j))
			}
			if action.Field == "" {
				return reconciler.NewValidationError(fmt.Sprintf("spec.rules[%d].actions[%d].field cannot be empty", i, j))
			}
		}
	}

	return nil
}

// verifyTargetRef checks if a target reference exists in the cluster
func (r *Reconciler) verifyTargetRef(ctx context.Context, namespace string, targetRef xv1alpha1.TargetRef) xv1alpha1.TargetRefStatus {
	status := xv1alpha1.TargetRefStatus{
		Kind:    targetRef.Kind,
		Name:    targetRef.Name,
		Applied: false,
	}

	// TODO: Implement the actual verification of the target reference
	status.Applied = true
	status.Message = "Target reference validated"

	return status
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("ctrlr").WithName("manifestcustomization")

	mainObjectHandler := wrapEventHandler(logger, &handler.EnqueueRequestForObject{})

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			LogConstructor: func(req *reconcile.Request) logr.Logger {
				log := logger
				if req != nil {
					log = log.WithValues("ManifestCustomization", req.NamespacedName)
				}
				return log
			},
		}).
		Watches(&xv1alpha1.ManifestCustomization{}, mainObjectHandler).Named("manifestcustomization").
		Complete(reconciler.NewStandardReconcilerWithFinalizer[*xv1alpha1.ManifestCustomization](r.Client, r.Reconcile, r.Finalize, constants.FinalizerName))
}

func (r *Reconciler) determineStatus(ctx context.Context, mc *xv1alpha1.ManifestCustomization, reconcileErr error) (xv1alpha1.ManifestCustomizationStatus, error) {
	var errs errlist.Builder
	reconciledCondition := r.determineReconciledCondition(reconcileErr)

	status := *mc.Status.DeepCopy()
	status.ObservedGeneration = mc.Generation
	status.SetCondition(reconciledCondition)
	status.State = deriveState(reconciledCondition)

	// Update target ref statuses
	targetStatuses := make([]xv1alpha1.TargetRefStatus, 0, len(mc.Spec.TargetRefs))
	for _, targetRef := range mc.Spec.TargetRefs {
		targetStatus := r.verifyTargetRef(ctx, mc.Namespace, targetRef)
		targetStatuses = append(targetStatuses, targetStatus)
	}
	status.TargetRefStatuses = targetStatuses

	return status, errs.Error()
}

func (r *Reconciler) updateStatus(ctx context.Context, mc *xv1alpha1.ManifestCustomization, reconcileErr error) error {
	var errs errlist.Builder

	status, err := r.determineStatus(ctx, mc, reconcileErr)
	if err != nil {
		errs.Add(fmt.Errorf("failed to determine status: %w", err))
	}

	if !reflect.DeepEqual(mc.Status, status) {
		if err := r.Client.Status().Patch(ctx, mc, kube.NewStatusPatch(status)); err != nil {
			errs.Add(fmt.Errorf("failed to patch status: %w", err))
		}
	}
	return errs.Error()
}

func deriveState(reconciledCondition xv1alpha1.ManifestCustomizationCondition) xv1alpha1.ManifestCustomizationConditionReason {
	if reconciledCondition.Status != metav1.ConditionTrue {
		return reconciledCondition.Reason
	}
	return xv1alpha1.ManifestCustomizationReasonHealthy
}

func (r *Reconciler) determineReconciledCondition(err error) xv1alpha1.ManifestCustomizationCondition {
	c := xv1alpha1.ManifestCustomizationCondition{Type: xv1alpha1.ManifestCustomizationConditionReconciled}

	if err == nil {
		c.Status = metav1.ConditionTrue
		c.Reason = xv1alpha1.ManifestCustomizationReasonHealthy
		c.Message = "ManifestCustomization reconciled successfully"
	} else {
		c.Status = metav1.ConditionFalse
		c.Reason = xv1alpha1.ManifestCustomizationReasonReconcileError
		c.Message = fmt.Sprintf("error reconciling ManifestCustomization: %v", err)
	}

	return c
}

func wrapEventHandler(logger logr.Logger, handler handler.EventHandler) handler.EventHandler {
	return enqueuelogger.WrapIfNecessary("ManifestCustomization", logger, handler)
}
