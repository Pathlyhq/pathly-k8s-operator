package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pathlyv1 "github.com/pathlyhq/pathly-k8s-operator/api/v1alpha1"
)

// ConnectionReconciler validates that the referenced Secret holds a Pathly token.
type ConnectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pathly.pathlyhq.com,resources=pathlyconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pathly.pathlyhq.com,resources=pathlyconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	conn := &pathlyv1.PathlyConnection{}
	if err := r.Get(ctx, req.NamespacedName, conn); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	_, err := tokenFromSecret(ctx, r.Client, conn.Namespace, conn.Spec.SecretRef)
	ready := err == nil
	msg := "token Secret readable"
	if err != nil {
		msg = err.Error()
		logger.Info("PathlyConnection not ready", "error", msg)
	}

	conn.Status.Ready = ready
	conn.Status.Message = msg
	conn.Status.ObservedGeneration = conn.Generation
	if err := r.Status().Update(ctx, conn); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pathlyv1.PathlyConnection{}).
		Complete(r)
}

// ensureFinalizer adds the Pathly finalizer when missing.
func ensureFinalizer(obj client.Object) bool {
	if controllerutil.ContainsFinalizer(obj, finalizerName) {
		return false
	}
	controllerutil.AddFinalizer(obj, finalizerName)
	return true
}

func removeFinalizer(obj client.Object) {
	controllerutil.RemoveFinalizer(obj, finalizerName)
}

func statusMessage(err error) string {
	if err == nil {
		return "synced"
	}
	return fmt.Sprintf("%v", err)
}
