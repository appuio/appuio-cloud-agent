package userinfo

import (
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
)

const (
	// saPrefix represents the service account prefix in admission requests
	saPrefix = "system:serviceaccount:"

	clusterRoleKind = "ClusterRole"
	roleKind        = "Role"
)

// RoleRefs gets the list of roles and cluster roles for the given user information.
// Only cluster roles bound by a cluster role binding are returned.
// Role bindings are ignored.
func ClusterRoleRefs(crbLister rbacv1listers.ClusterRoleBindingLister, user authenticationv1.UserInfo) (clusterroles []string, err error) {
	clusterroleBindings, err := crbLister.List(labels.NewSelector())
	if err != nil {
		return nil, fmt.Errorf("failed to list clusterrolebindings: %v", err)
	}
	return clusterRoleRefs(clusterroleBindings, user), nil
}

func clusterRoleRefs(clusterroleBindings []*rbacv1.ClusterRoleBinding, userInfo authenticationv1.UserInfo) (clusterRoles []string) {
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
