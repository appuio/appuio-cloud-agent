package controllers

import (
	"context"
	"encoding/json"

	controlv1 "github.com/appuio/control-api/apis/v1"
	userv1 "github.com/openshift/api/user/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// UserAttributeSyncReconciler reconciles a User object
type UserAttributeSyncReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ForeignClient client.Client
}

const DefaultOrganizationAnnotation = "appuio.io/default-organization"

//+kubebuilder:rbac:groups=user.openshift.io,resources=users,verbs=get;list;watch;update;patch

// Reconcile syncs the User with the upstream User resource from the foreign (Control-API) cluster.
// Currently the following attributes are synced:
// - .spec.preferences.defaultOrganizationRef -> .metadata.annotations["appuio.io/default-organization"]
func (r *UserAttributeSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling User")

	var upstream controlv1.User
	if err := r.ForeignClient.Get(ctx, client.ObjectKey{Name: req.Name}, &upstream); err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("Upstream user not found")
			return ctrl.Result{}, nil
		}
		l.Error(err, "unable to get upstream User")
		return ctrl.Result{}, err
	}

	var local userv1.User
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &local); err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("Local user not found")
			return ctrl.Result{}, nil
		}
		l.Error(err, "unable to get local User")
		return ctrl.Result{}, err
	}

	if local.Annotations != nil && local.Annotations[DefaultOrganizationAnnotation] == upstream.Spec.Preferences.DefaultOrganizationRef {
		l.Info("User has correct default organization annotation")
		return ctrl.Result{}, nil
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				DefaultOrganizationAnnotation: upstream.Spec.Preferences.DefaultOrganizationRef,
			},
		},
	}
	encPatch, err := json.Marshal(patch)
	if err != nil {
		l.Error(err, "unable to marshal patch")
		return ctrl.Result{}, err
	}

	if err := r.Client.Patch(ctx, &local, client.RawPatch(types.StrategicMergePatchType, encPatch)); err != nil {
		l.Error(err, "unable to patch User")
		return ctrl.Result{}, err
	}

	// Record event so we don't trigger another reconcile loop but still know when the last sync happened.
	r.Recorder.Eventf(&local, "Normal", "Reconciled", "Reconciled User")
	l.Info("User reconciled")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserAttributeSyncReconciler) SetupWithManagerAndForeignCluster(mgr ctrl.Manager, foreign cluster.Cluster) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&userv1.User{}).
		WatchesRawSource(source.Kind(foreign.GetCache(), &controlv1.User{}, &handler.TypedEnqueueRequestForObject[*controlv1.User]{})).
		Complete(r)
}
