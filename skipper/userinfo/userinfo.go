package userinfo

import (
	"context"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch

const (
	// saPrefix represents the service account prefix in admission requests
	saPrefix = "system:serviceaccount:"

	clusterRoleKind = "ClusterRole"
)

// ClusterRoleRefsForUser gets the list of roles and cluster roles for the given user information.
// Only cluster roles bound by a cluster role binding are returned.
// Role bindings are ignored.
func ClusterRoleRefsForUser(ctx context.Context, cli client.Reader, user authenticationv1.UserInfo) (clusterroles []string, err error) {
	crbs, err := listClusterRoles(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("failed to list clusterrolebindings: %v", err)
	}
	return clusterRoleRefs(crbs, user), nil
}

func clusterRoleRefs(clusterroleBindings []rbacv1.ClusterRoleBinding, userInfo authenticationv1.UserInfo) (clusterRoles []string) {
	for _, clusterRoleBinding := range clusterroleBindings {
		for _, subject := range clusterRoleBinding.Subjects {
			if clusterRoleBinding.RoleRef.Kind == clusterRoleKind && matchSubject(subject, userInfo) {
				clusterRoles = append(clusterRoles, clusterRoleBinding.RoleRef.Name)
			}
		}
	}
	return clusterRoles
}

func matchSubject(subject rbacv1.Subject, userInfo authenticationv1.UserInfo) bool {
	switch subject.Kind {
	case rbacv1.ServiceAccountKind:
		return userInfo.Username == saPrefix+subject.Namespace+":"+subject.Name
	case rbacv1.UserKind:
		return userInfo.Username == subject.Name
	case rbacv1.GroupKind:
		for _, group := range userInfo.Groups {
			if subject.Name == group {
				return true
			}
		}
		return false
	}

	return false
}

func listClusterRoles(ctx context.Context, cli client.Reader) ([]rbacv1.ClusterRoleBinding, error) {
	var crbs rbacv1.ClusterRoleBindingList
	if err := cli.List(ctx, &crbs); err != nil {
		return nil, err
	}
	return crbs.Items, nil
}
