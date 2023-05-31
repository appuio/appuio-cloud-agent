package controllers

import (
	"context"

	controlv1 "github.com/appuio/control-api/apis/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/controllers/clustersource"
)

// ZoneUsageProfileSyncReconciler reconciles a ZoneUsageProfile object
type ZoneUsageProfileSyncReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ForeignClient client.Client
}

//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles/finalizers,verbs=update

// Reconcile syncs the ZoneUsageProfile with the upstream UsageProfile resource from the foreign (Control-API) cluster.
func (r *ZoneUsageProfileSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling ZoneUsageProfile")

	var upstream controlv1.UsageProfile
	if err := r.ForeignClient.Get(ctx, client.ObjectKey{Name: req.Name}, &upstream); err != nil {
		l.Error(err, "unable to get upstream UsageProfile")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	zoneUsageProfile := &cloudagentv1.ZoneUsageProfile{ObjectMeta: metav1.ObjectMeta{Name: upstream.Name}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, zoneUsageProfile, func() error {
		zoneUsageProfile.Spec.UpstreamSpec = upstream.Spec
		return nil
	})
	if err != nil {
		l.Error(err, "unable to create or update ZoneUsageProfile")
		return ctrl.Result{}, err
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(zoneUsageProfile), zoneUsageProfile); err != nil {
		l.Error(err, "unable to get ZoneUsageProfile after update")
		return ctrl.Result{}, err
	}
	// Record event so we don't trigger another reconcile loop but still know when the last sync happened.
	r.Recorder.Eventf(zoneUsageProfile, "Normal", "Reconciled", "Reconciled ZoneUsageProfile: %s", op)
	l.Info("ZoneUsageProfile reconciled", "operation", op)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneUsageProfileSyncReconciler) SetupWithManagerAndForeignCluster(mgr ctrl.Manager, foreign clustersource.ClusterSource) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudagentv1.ZoneUsageProfile{}).
		WatchesRawSource(foreign.SourceFor(&controlv1.UsageProfile{}), &handler.EnqueueRequestForObject{}).
		Complete(r)
}
