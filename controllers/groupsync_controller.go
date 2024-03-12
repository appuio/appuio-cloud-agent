package controllers

import (
	"context"
	"fmt"
	"slices"
	"strings"

	controlv1 "github.com/appuio/control-api/apis/v1"
	userv1 "github.com/openshift/api/user/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/appuio/appuio-cloud-agent/controllers/clustersource"
)

// GroupSyncReconciler reconciles a Group object
type GroupSyncReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ForeignClient client.Client

	ControlAPIFinalizerZoneName string
}

// OrganizationMembersManifestName is the static name of the OrganizationMembers manifest
// in the control-api cluster.
const OrganizationMembersManifestName = "members"

const UpstreamFinalizerPrefix = "agent.appuio.io/group-zone-"

//+kubebuilder:rbac:groups=user.openshift.io,resources=groups,verbs=get;list;watch;update;patch;create;delete

// Reconcile syncs the Group with the upstream OrganizationMembers or Team resource from the foreign (Control-API) cluster.
func (r *GroupSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling Group")

	finalizerName := UpstreamFinalizerPrefix + r.ControlAPIFinalizerZoneName

	var members []controlv1.UserRef
	var upstream client.Object

	isTeam := strings.ContainsRune(req.Name, '+')
	if isTeam {
		nsn := strings.SplitN(req.Name, "+", 2)
		ns, name := nsn[0], nsn[1]
		var u controlv1.Team
		if err := r.ForeignClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &u); err != nil {
			if apierrors.IsNotFound(err) {
				l.Info("Upstream team not found")
				return ctrl.Result{}, nil
			}
			l.Error(err, "unable to get upstream Team")
			return ctrl.Result{}, err
		}
		upstream = &u
		members = u.Status.ResolvedUserRefs
	} else {
		var u controlv1.OrganizationMembers
		if err := r.ForeignClient.Get(ctx, client.ObjectKey{Namespace: req.Name, Name: OrganizationMembersManifestName}, &u); err != nil {
			if apierrors.IsNotFound(err) {
				l.Info("Upstream organization members not found")
				return ctrl.Result{}, nil
			}
			l.Error(err, "unable to get upstream OrganizationMembers")
			return ctrl.Result{}, err
		}
		upstream = &u
		members = u.Status.ResolvedUserRefs
	}

	group := &userv1.Group{ObjectMeta: metav1.ObjectMeta{Name: req.Name}}

	if upstream.GetDeletionTimestamp() != nil {
		l.Info("Upstream Group is being deleted")

		err := r.Delete(ctx, group)
		if err != nil && !apierrors.IsNotFound(err) {
			l.Error(err, "unable to delete Group")
			return ctrl.Result{}, err
		}

		l.Info("Group deleted")

		if controllerutil.RemoveFinalizer(upstream, finalizerName) {
			if err := r.ForeignClient.Update(ctx, upstream); err != nil {
				l.Error(err, "unable to remove finalizer from upstream")
				return ctrl.Result{}, err
			}
		}

		l.Info("Finalizer removed from upstream", "finalizer", finalizerName)

		return ctrl.Result{}, nil
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, group, func() error {
		group.Users = make([]string, len(members))
		for i, member := range members {
			group.Users[i] = member.Name
		}
		slices.Sort(group.Users)
		return nil
	})
	if err != nil {
		l.Error(err, "unable to create or update (%q) Group", op)
		return ctrl.Result{}, err
	}
	l.Info("Group reconciled", "operation", op)

	if controllerutil.AddFinalizer(upstream, finalizerName) {
		if err := r.ForeignClient.Update(ctx, upstream); err != nil {
			l.Error(err, "unable to add finalizer to upstream")
			return ctrl.Result{}, err
		}
		l.Info("Finalizer added to upstream", "finalizer", finalizerName)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupSyncReconciler) SetupWithManagerAndForeignCluster(mgr ctrl.Manager, foreign clustersource.ClusterSource) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&userv1.Group{}).
		WatchesRawSource(foreign.SourceFor(&controlv1.Team{}), handler.EnqueueRequestsFromMapFunc(teamMapper)).
		WatchesRawSource(foreign.SourceFor(&controlv1.OrganizationMembers{}), handler.EnqueueRequestsFromMapFunc(organizationMembersMapper)).
		Complete(r)
}

// teamMapper maps the combination of namespace and name of the manifest as the group name to reconcile.
// The namespace is the organization for the teams.
func teamMapper(ctx context.Context, o client.Object) []reconcile.Request {
	team, ok := o.(*controlv1.Team)
	if !ok {
		log.FromContext(ctx).Error(nil, "expected a Team object got a %T", o)
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: fmt.Sprintf("%s+%s", team.Namespace, team.Name)}},
	}
}

// organizationMembersMapper maps the namespace of the manifest as the group name to reconcile.
// The name is static and the organization is in the namespace field.
func organizationMembersMapper(ctx context.Context, o client.Object) []reconcile.Request {
	member, ok := o.(*controlv1.OrganizationMembers)
	if !ok {
		log.FromContext(ctx).Error(nil, "expected a OrganizationMembers object got a %T", o)
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: member.Namespace}},
	}
}
