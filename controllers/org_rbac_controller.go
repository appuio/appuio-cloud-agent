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

const LabelRoleBindingUninitiliazied = "appuio.io/uninitialized"

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;patch;update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reacts to pod updates and emits events if the fair use request ratio is violated
func (r *OrganizationRBACReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := req.Name
	l := log.FromContext(ctx).WithValues("namespace", namespace)

	org, err := r.getOrganization(ctx, namespace)
	if err != nil {
		l.Error(err, "unable to get namespace")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if org == "" {
		return ctrl.Result{}, nil
	}

	rbState, err := r.getRoleBindingStates(ctx, namespace)
	if err != nil {
		l.Error(err, "unable to list rolebindings in namespace")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for rb, cr := range r.DefaultClusterRoles {
		_, initialized := rbState[rb]
		if !initialized {
			if err := r.putRoleBinding(ctx, rb, namespace, cr, org); err != nil {
				l.WithValues("rolebinding", rb).Error(err, "unable to create rolebinding")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *OrganizationRBACReconciler) getOrganization(ctx context.Context, namespace string) (string, error) {
	var ns corev1.Namespace
	if err := r.Get(ctx, client.ObjectKey{Name: namespace}, &ns); err != nil {
		return "", err
	}
	org := ""
	nsLabels := ns.Labels
	if nsLabels != nil {
		org = nsLabels[r.OrganizationLabel]
	}
	return org, nil
}

func (r *OrganizationRBACReconciler) getRoleBindingStates(ctx context.Context, namespace string) (map[string]struct{}, error) {
	var rolebindings rbacv1.RoleBindingList
	if err := r.List(ctx, &rolebindings, client.InNamespace(namespace)); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	rbState := map[string]struct{}{}
	for _, rb := range rolebindings.Items {
		if !rolebindingIsUninitialized(rb) {
			rbState[rb.Name] = struct{}{}
		}
	}
	return rbState, nil
}

func rolebindingIsUninitialized(rolebinding rbacv1.RoleBinding) bool {
	if rolebinding.Labels == nil {
		return false
	}
	res, err := strconv.ParseBool(rolebinding.Labels[LabelRoleBindingUninitiliazied])
	return res && err == nil
}

func (r *OrganizationRBACReconciler) putRoleBinding(ctx context.Context, name, namespace string, clusterRole string, group string) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:     "Group",
				APIGroup: "rbac.authorization.k8s.io",
				Name:     group,
			},
		}
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole,
		}
		delete(rb.Labels, LabelRoleBindingUninitiliazied)
		return nil
	})
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrganizationRBACReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}
