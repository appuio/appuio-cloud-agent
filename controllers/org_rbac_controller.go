package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// OrganizationRBACReconciler reconciles RBAC rules for organization namespaces
type OrganizationRBACReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme

	// OrganizationLabel is the label that marks to what organization (if any) the namespace belongs to
	OrganizationLabel string
	// DefaultClusterRoles is a map where the keys are the name of default rolebindings to create and the values are the names of the clusterroles they bind to
	DefaultClusterRoles map[string]string
}

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;patch;update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reacts to pod updates and emits events if the fair use request ratio is violated
func (r *OrganizationRBACReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrganizationRBACReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}
