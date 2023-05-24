package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudagentappuioiov1 "github.com/appuio/appuio-cloud-agent/api/v1"
)

// ZoneUsageProfileReconciler reconciles a ZoneUsageProfile object
type ZoneUsageProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ZoneUsageProfile object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ZoneUsageProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneUsageProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudagentappuioiov1.ZoneUsageProfile{}).
		Complete(r)
}
