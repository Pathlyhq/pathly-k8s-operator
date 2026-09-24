package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pathlyv1 "github.com/pathlyhq/pathly-k8s-operator/api/v1alpha1"
	pathlyclient "github.com/pathlyhq/pathly-k8s-operator/internal/client"
)

// PathlyClientFactory builds an API client from a PathlyConnection name.
type PathlyClientFactory func(ctx context.Context, kube client.Client, namespace, connectionName string) (*pathlyclient.Client, error)

func resolveClient(factory PathlyClientFactory, ctx context.Context, kube client.Client, namespace, connectionName string) (*pathlyclient.Client, error) {
	if factory != nil {
		return factory(ctx, kube, namespace, connectionName)
	}
	return ClientFromConnection(ctx, kube, namespace, corev1.LocalObjectReference{Name: connectionName})
}

// ScenarioReconciler reconciles PathlyScenario objects against the Pathly API.
type ScenarioReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	NewClient PathlyClientFactory
}

// +kubebuilder:rbac:groups=pathly.pathlyhq.com,resources=pathlyscenarios,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pathly.pathlyhq.com,resources=pathlyscenarios/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pathly.pathlyhq.com,resources=pathlyscenarios/finalizers,verbs=update

func (r *ScenarioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cr := &pathlyv1.PathlyScenario{}
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
			if err := cli.DeleteScenario(ctx, cr.Status.ID); err != nil && !pathlyclient.IsNotFound(err) {
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
		in := scenarioInput(cr.Spec)
		got, err := cli.CreateScenario(ctx, in, idempotencyKey(cr.UID, "create"))
		if err != nil {
			return r.failStatus(ctx, cr, err)
		}
		cr.Status.ID = got.ID
		applyScenarioObservation(&cr.Status, got)
		cr.Status.Ready = true
		cr.Status.Message = "created"
		cr.Status.ObservedGeneration = cr.Generation
		if err := r.Status().Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("created Pathly scenario", "id", got.ID)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	got, err := cli.GetScenario(ctx, cr.Status.ID)
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

	if !scenarioUpToDate(cr.Spec, got) {
		in := scenarioInput(cr.Spec)
		got, err = cli.UpdateScenario(ctx, cr.Status.ID, in)
		if err != nil {
			return r.failStatus(ctx, cr, err)
		}
		logger.Info("updated Pathly scenario", "id", cr.Status.ID)
	}

	applyScenarioObservation(&cr.Status, got)
	cr.Status.Ready = true
	cr.Status.Message = "synced"
	cr.Status.ObservedGeneration = cr.Generation
	if err := r.Status().Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *ScenarioReconciler) failStatus(ctx context.Context, cr *pathlyv1.PathlyScenario, err error) (ctrl.Result, error) {
	cr.Status.Ready = false
	cr.Status.Message = statusMessage(err)
	cr.Status.ObservedGeneration = cr.Generation
	_ = r.Status().Update(ctx, cr)
	return ctrl.Result{}, err
}

func scenarioInput(p pathlyv1.PathlyScenarioSpec) pathlyclient.ScenarioInput {
	typ := p.Type
	if typ == "" {
		typ = "http"
	}
	name := p.Name
	return pathlyclient.ScenarioInput{
		Name:           &name,
		Type:           &typ,
		URL:            CopyStringPtr(p.URL),
		IntervalSec:    CopyInt64Ptr(p.IntervalSec),
		Method:         CopyStringPtr(p.Method),
		ExpectedStatus: CopyInt64Ptr(p.ExpectedStatus),
		MaxLatencyMs:   CopyInt64Ptr(p.MaxLatencyMs),
		ExpectText:     CopyStringPtr(p.ExpectText),
		Regions:        append([]string(nil), p.Regions...),
		Tags:           append([]string(nil), p.Tags...),
		Folder:         CopyStringPtr(p.Folder),
		Severity:       CopyStringPtr(p.Severity),
		Runbook:        CopyStringPtr(p.Runbook),
		Cron:           CopyStringPtr(p.Cron),
		Enabled:        CopyBoolPtr(p.Enabled),
	}
}

func scenarioUpToDate(want pathlyv1.PathlyScenarioSpec, got *pathlyclient.Scenario) bool {
	if got.Name != want.Name {
		return false
	}
	if want.URL != nil && (got.URL == nil || *got.URL != *want.URL) {
		return false
	}
	if want.Enabled != nil && (got.Enabled == nil || *got.Enabled != *want.Enabled) {
		return false
	}
	if want.IntervalSec != nil && (got.IntervalSec == nil || *got.IntervalSec != *want.IntervalSec) {
		return false
	}
	if want.Method != nil && (got.Method == nil || *got.Method != *want.Method) {
		return false
	}
	if want.ExpectedStatus != nil && (got.ExpectedStatus == nil || *got.ExpectedStatus != *want.ExpectedStatus) {
		return false
	}
	return true
}

func applyScenarioObservation(st *pathlyv1.PathlyScenarioStatus, got *pathlyclient.Scenario) {
	st.ID = got.ID
	st.LastStatus = CopyStringPtr(got.LastStatus)
	st.ScenarioFingerprint = CopyStringPtr(got.ScenarioFingerprint)
	st.CreatedAt = CopyStringPtr(got.CreatedAt)
	st.MutedUntil = CopyStringPtr(got.MutedUntil)
}

func (r *ScenarioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pathlyv1.PathlyScenario{}).
		Complete(r)
}
