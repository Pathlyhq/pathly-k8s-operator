package controller

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pathlyv1 "github.com/pathlyhq/pathly-k8s-operator/api/v1alpha1"
	pathlyclient "github.com/pathlyhq/pathly-k8s-operator/internal/client"
)

// SlaTargetReconciler reconciles PathlySlaTarget objects against the Pathly API.
type SlaTargetReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	NewClient PathlyClientFactory
}

func (r *SlaTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cr := &pathlyv1.PathlySlaTarget{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if cr.DeletionTimestamp.IsZero() {
		if ensureFinalizer(cr) {
			if err := r.Update(ctx, cr); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	cli, err := resolveClient(r.NewClient, ctx, r.Client, cr.Namespace, cr.Spec.ConnectionRef.Name)
	if err != nil {
		return r.failStatus(ctx, cr, err)
	}

	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ID != "" {
			if err := cli.DeleteSlaTarget(ctx, cr.Status.ID); err != nil && !pathlyclient.IsNotFound(err) {
				return r.failStatus(ctx, cr, err)
			}
		}
		removeFinalizer(cr)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	got, err := cli.UpsertSlaTarget(ctx, pathlyclient.SlaTargetInput{
		MonitorID:          CopyStringPtr(cr.Spec.MonitorID),
		Name:               CopyStringPtr(cr.Spec.Name),
		ObjectivePct:       cr.Spec.ObjectivePct,
		WindowDays:         cr.Spec.WindowDays,
		ExcludeMaintenance: CopyBoolPtr(cr.Spec.ExcludeMaintenance),
		WarnAtBudgetRatio:  CopyFloat64Ptr(cr.Spec.WarnAtBudgetRatio),
		Enabled:            CopyBoolPtr(cr.Spec.Enabled),
	}, idempotencyKey(cr.UID, "upsert"))
	if err != nil {
		return r.failStatus(ctx, cr, err)
	}

	applySlaObservation(&cr.Status, got)
	cr.Status.Ready = true
	cr.Status.Message = "synced"
	cr.Status.ObservedGeneration = cr.Generation
	if err := r.Status().Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("synced Pathly SLA target", "id", got.ID)
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *SlaTargetReconciler) failStatus(ctx context.Context, cr *pathlyv1.PathlySlaTarget, err error) (ctrl.Result, error) {
	cr.Status.Ready = false
	cr.Status.Message = statusMessage(err)
	cr.Status.ObservedGeneration = cr.Generation
	_ = r.Status().Update(ctx, cr)
	return ctrl.Result{}, err
}

func applySlaObservation(st *pathlyv1.PathlySlaTargetStatus, got *pathlyclient.SlaTarget) {
	st.ID = got.ID
	st.MonitorID = CopyStringPtr(got.MonitorID)
	st.Name = CopyStringPtr(got.Name)
	st.ObjectivePct = CopyFloat64Ptr(got.ObjectivePct)
	st.WindowDays = CopyInt64Ptr(got.WindowDays)
	st.ExcludeMaintenance = CopyBoolPtr(got.ExcludeMaintenance)
	st.WarnAtBudgetRatio = CopyFloat64Ptr(got.WarnAtBudgetRatio)
	st.Enabled = CopyBoolPtr(got.Enabled)
}

func (r *SlaTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pathlyv1.PathlySlaTarget{}).
		Complete(r)
}
