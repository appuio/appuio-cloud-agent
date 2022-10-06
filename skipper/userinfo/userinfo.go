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
// Roles and cluster roles bound through RoleBindings are prefixed with the namespace the binding is in.
func RoleRefs(rbLister rbacv1listers.RoleBindingLister, crbLister rbacv1listers.ClusterRoleBindingLister, user authenticationv1.UserInfo) (roles []string, clusterroles []string, err error) {
	roleBindings, err := rbLister.List(labels.Everything())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list rolebindings: %v", err)
	}
	rs, crs := roleRefs(roleBindings, user)

	clusterroleBindings, err := crbLister.List(labels.NewSelector())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list clusterrolebindings: %v", err)
	}
	crs = append(crs, clusterRoleRefs(clusterroleBindings, user)...)
	return rs, crs, nil
}

func roleRefs(roleBindings []*rbacv1.RoleBinding, userInfo authenticationv1.UserInfo) (roles []string, clusterRoles []string) {
	for _, rolebinding := range roleBindings {
		for _, subject := range rolebinding.Subjects {
			if matchSubject(subject, userInfo, rolebinding.Namespace) {
				name := rolebinding.Namespace + ":" + rolebinding.RoleRef.Name
				switch rolebinding.RoleRef.Kind {
				case roleKind:
					roles = append(roles, name)
				case clusterRoleKind:
					clusterRoles = append(clusterRoles, name)
				}
			}
		}
	}
	return roles, clusterRoles
}

func clusterRoleRefs(clusterroleBindings []*rbacv1.ClusterRoleBinding, userInfo authenticationv1.UserInfo) (clusterRoles []string) {
	for _, clusterRoleBinding := range clusterroleBindings {
		for _, subject := range clusterRoleBinding.Subjects {
			if clusterRoleBinding.RoleRef.Kind == clusterRoleKind {
				if matchSubject(subject, userInfo, subject.Namespace) {
					clusterRoles = append(clusterRoles, clusterRoleBinding.RoleRef.Name)
				}
			}
		}
	}
	return clusterRoles
}

func matchSubject(subject rbacv1.Subject, userInfo authenticationv1.UserInfo, namespace string) bool {
	switch subject.Kind {
	case rbacv1.ServiceAccountKind:
		return userInfo.Username == saPrefix+namespace+":"+subject.Name
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
