package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	controlv1 "github.com/appuio/control-api/apis/v1"
	configv1 "github.com/openshift/api/config/v1"

	"go.uber.org/multierr"
)

type ZoneK8sVersionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ForeignClient client.Client
	RESTClient    rest.Interface

	// upstream zone ID. The agent expects that the control-api zone
	// object is labeled with
	ZoneID string
}

const (
	upstreamZoneIdentifierLabelKey = "control.appuio.io/zone-cluster-id"
	kubernetesVersionFeatureKey    = "kubernetesVersion"
	openshiftVersionFeatureKey     = "openshiftVersion"
)

// Reconcile reads the K8s and OCP versions and writes them to the upstream
// zone
// The logic in this reconcile function is adapted from
// https://github.com/projectsyn/steward
func (r *ZoneK8sVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling zone K8s version")

	var cv = configv1.ClusterVersion{}
	if err := r.Client.Get(ctx, req.NamespacedName, &cv); err != nil {
		return ctrl.Result{}, err
	}

	ocpVersion, err := extractOpenShiftVersion(&cv)
	if err != nil {
		return ctrl.Result{}, err
	}

	l.Info("OCP current version", "version", ocpVersion)

	// We don't use client-go's ServerVersion() so we get context support
	body, err := r.RESTClient.Get().AbsPath("/version").Do(ctx).Raw()
	if err != nil {
		return ctrl.Result{}, err
	}
	var k8sVersion version.Info
	err = json.Unmarshal(body, &k8sVersion)
	if err != nil {
		return ctrl.Result{}, err
	}
	l.Info("K8s current version", "version", k8sVersion)

	// List zones by label because we don't enforce any naming conventions
	// for the Zone objects on the control-api cluster.
	var zones = controlv1.ZoneList{}
	if err := r.ForeignClient.List(ctx, &zones, client.MatchingLabels{upstreamZoneIdentifierLabelKey: r.ZoneID}); err != nil {
		return ctrl.Result{}, err
	}

	if len(zones.Items) == 0 {
		l.Info("No upstream zone found", "zone ID", r.ZoneID)
		return ctrl.Result{}, nil
	}

	if len(zones.Items) > 1 {
		l.Info("Multiple upstream zones found, updating all", "zone ID", r.ZoneID)
	}

	var errs []error
	for _, z := range zones.Items {
		z.Data.Features[kubernetesVersionFeatureKey] =
			fmt.Sprintf("%s.%s", k8sVersion.Major, k8sVersion.Minor)
		z.Data.Features[openshiftVersionFeatureKey] =
			fmt.Sprintf("%s.%s", ocpVersion.Major, ocpVersion.Minor)
		if err := r.ForeignClient.Update(ctx, &z); err != nil {
			errs = append(errs, err)
		}
	}

	return ctrl.Result{}, multierr.Combine(errs...)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneK8sVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configv1.ClusterVersion{}).
		Named("zone_k8s_version").
		Complete(r)
}

// extract version of latest completed and verified upgrade from the OCP ClusterVersion resource.
func extractOpenShiftVersion(cv *configv1.ClusterVersion) (*version.Info, error) {
	currentVersion := ""
	lastUpdate := time.Time{}
	for _, h := range cv.Status.History {
		if h.State == "Completed" && h.Verified == true && h.CompletionTime.Time.After(lastUpdate) {
			currentVersion = h.Version
			lastUpdate = h.CompletionTime.Time
		}
	}
	if currentVersion == "" {
		currentVersion = cv.Status.Desired.Version
	}

	if currentVersion == "" {
		return nil, fmt.Errorf("Unable to extract current OpenShift version")
	}

	verparts := strings.Split(currentVersion, ".")
	version := version.Info{
		Major: verparts[0],
		Minor: verparts[1],
	}
	return &version, nil
}
