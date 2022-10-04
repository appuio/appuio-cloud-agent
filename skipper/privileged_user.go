package skipper

import (
	"github.com/appuio/appuio-cloud-agent/skipper/userinfo"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ Skipper = &PrivilegedUserSkipper{}

// PrivilegedUserSkipper skips request validations for privileged users.
type PrivilegedUserSkipper struct {
	RoleBindingLister        rbacv1listers.RoleBindingLister
	ClusterRoleBindingLister rbacv1listers.ClusterRoleBindingLister

	PrivilegedGroups       []string
	PrivilegedUsers        []string
	PrivilegedRoles        []string
	PrivilegedClusterRoles []string
}

func (s *PrivilegedUserSkipper) Skip(req admission.Request) (bool, error) {
	for _, ag := range s.PrivilegedGroups {
		for _, ug := range req.UserInfo.Groups {
			if ug == ag {
				return true, nil
			}
		}
	}

	for _, au := range s.PrivilegedUsers {
		if req.UserInfo.Username == au {
			return true, nil
		}
	}

	roles, clusterroles, err := userinfo.RoleRefs(s.RoleBindingLister, s.ClusterRoleBindingLister, req.UserInfo)
	if err != nil {
		return false, err
	}

	for _, acr := range s.PrivilegedClusterRoles {
		for _, cr := range clusterroles {
			if cr == acr {
				return true, nil
			}
		}
	}

	for _, acr := range s.PrivilegedRoles {
		for _, cr := range roles {
			if cr == acr {
				return true, nil
			}
		}
	}

	return false, nil
}
