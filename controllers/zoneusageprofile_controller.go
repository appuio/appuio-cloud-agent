package controllers

import (
	"context"

	controlv1 "github.com/appuio/control-api/apis/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/controllers/clustersource"
)

// ZoneUsageProfileReconciler reconciles a ZoneUsageProfile object
type ZoneUsageProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	ForeignClient client.Client
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
	l := log.FromContext(ctx)
	l.Info("Reconciling ZoneUsageProfile")

	var upstream controlv1.UsageProfile
	if err := r.ForeignClient.Get(ctx, client.ObjectKey{Name: req.Name}, &upstream); err != nil {
		l.Error(err, "unable to get upstream UsageProfile")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	zoneUsageProfile := cloudagentv1.ZoneUsageProfile{ObjectMeta: metav1.ObjectMeta{Name: upstream.Name}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, &zoneUsageProfile, func() error {
		zoneUsageProfile.Spec.UpstreamSpec = upstream.Spec
		return nil
	})
	if err != nil {
		l.Error(err, "unable to create or update ZoneUsageProfile")
		return ctrl.Result{}, err
	}

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		zoneUsageProfile.Status.LastSynced = metav1.Now()
		if err := r.Client.Status().Update(ctx, &zoneUsageProfile); err != nil {
			l.Error(err, "unable to update ZoneUsageProfile status")
			return ctrl.Result{}, err
		}
	}

	l.Info("ZoneUsageProfile reconciled", "operation", op)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneUsageProfileReconciler) SetupWithManagerAndForeignCluster(mgr ctrl.Manager, foreign clustersource.ClusterSource) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudagentv1.ZoneUsageProfile{}).
		WatchesRawSource(foreign.SourceFor(&controlv1.UsageProfile{}), &handler.EnqueueRequestForObject{}).
		Complete(r)
}
