package controllers

import (
	"context"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

const LabelRoleBindingUninitialized = "appuio.io/uninitialized"

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;patch;update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reacts to pod updates and emits events if the fair use request ratio is violated
func (r *OrganizationRBACReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("namespace", req.Name)

	var ns corev1.Namespace
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name}, &ns); err != nil {
		l.Error(err, "unable to get namespace")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	org := r.getOrganization(ns)
	if org == "" {
		return ctrl.Result{}, nil
	}

	for rb, cr := range r.DefaultClusterRoles {
		if err := r.putRoleBinding(ctx, ns, rb, cr, org); err != nil {
			l.WithValues("rolebinding", rb).Error(err, "unable to create rolebinding")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *OrganizationRBACReconciler) getOrganization(ns corev1.Namespace) string {
	org := ""
	nsLabels := ns.Labels
	if nsLabels != nil {
		org = nsLabels[r.OrganizationLabel]
	}
	return org
}

func (r *OrganizationRBACReconciler) putRoleBinding(ctx context.Context, ns corev1.Namespace, name string, clusterRole string, group string) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns.Name,
			Labels: map[string]string{
				LabelRoleBindingUninitialized: "true",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		if rolebindingIsUninitialized(rb) {
			rb.Subjects = []rbacv1.Subject{
				{
					Kind:     "Group",
					APIGroup: "rbac.authorization.k8s.io",
					Name:     group,
				},
			}
			delete(rb.Labels, LabelRoleBindingUninitialized)
		}
		controllerutil.SetControllerReference(&ns, rb, r.Scheme)
		return nil
	})

	return err
}

func rolebindingIsUninitialized(rolebinding *rbacv1.RoleBinding) bool {
	if rolebinding.Labels == nil {
		return false
	}
	res, err := strconv.ParseBool(rolebinding.Labels[LabelRoleBindingUninitialized])
	return res && err == nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrganizationRBACReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
