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

// WebhookReconciler reconciles PathlyWebhook objects against the Pathly API.
type WebhookReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	NewClient PathlyClientFactory
}

func (r *WebhookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cr := &pathlyv1.PathlyWebhook{}
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
			if err := cli.DeleteWebhook(ctx, cr.Status.ID); err != nil && !pathlyclient.IsNotFound(err) {
				return r.failStatus(ctx, cr, err)
			}
		}
		removeFinalizer(cr)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if cr.Status.ID == "" {
		got, err := cli.CreateWebhook(ctx, pathlyclient.WebhookInput{
			URL:    cr.Spec.URL,
			Events: append([]string(nil), cr.Spec.Events...),
		}, idempotencyKey(cr.UID, "create"))
		if err != nil {
			return r.failStatus(ctx, cr, err)
		}
		applyWebhookObservation(&cr.Status, got)
		cr.Status.Ready = true
		cr.Status.Message = "created"
		cr.Status.ObservedGeneration = cr.Generation
		if err := r.Status().Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("created Pathly webhook", "id", got.ID)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	got, err := cli.GetWebhook(ctx, cr.Status.ID)
	if err != nil {
		if pathlyclient.IsNotFound(err) {
			cr.Status.ID = ""
			cr.Status.Ready = false
			cr.Status.Message = "remote resource missing, will recreate"
			_ = r.Status().Update(ctx, cr)
			return ctrl.Result{Requeue: true}, nil
		}
		return r.failStatus(ctx, cr, err)
	}

	applyWebhookObservation(&cr.Status, got)
	cr.Status.Ready = true
	cr.Status.Message = "synced"
	cr.Status.ObservedGeneration = cr.Generation
	if err := r.Status().Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *WebhookReconciler) failStatus(ctx context.Context, cr *pathlyv1.PathlyWebhook, err error) (ctrl.Result, error) {
	cr.Status.Ready = false
	cr.Status.Message = statusMessage(err)
	cr.Status.ObservedGeneration = cr.Generation
	_ = r.Status().Update(ctx, cr)
	return ctrl.Result{}, err
}

func applyWebhookObservation(st *pathlyv1.PathlyWebhookStatus, got *pathlyclient.Webhook) {
	st.ID = got.ID
	st.Events = append([]string(nil), got.Events...)
	st.Enabled = CopyBoolPtr(got.Enabled)
	st.HasSecret = CopyBoolPtr(got.HasSecret)
	st.URLFingerprint = CopyStringPtr(got.URLFingerprint)
	st.CreatedAt = CopyStringPtr(got.CreatedAt)
}

func (r *WebhookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pathlyv1.PathlyWebhook{}).
		Complete(r)
}
