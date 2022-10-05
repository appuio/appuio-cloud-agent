package skipper

import (
	"github.com/minio/pkg/wildcard"
	kubeinformers "k8s.io/client-go/informers"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper/userinfo"
)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=get;list;watch

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

// NewPrivilegedUserSkipper creates a new PrivilegedUserSkipper with the *Lister set.
func NewPrivilegedUserSkipper(inf kubeinformers.SharedInformerFactory) *PrivilegedUserSkipper {
	return &PrivilegedUserSkipper{
		RoleBindingLister:        inf.Rbac().V1().RoleBindings().Lister(),
		ClusterRoleBindingLister: inf.Rbac().V1().ClusterRoleBindings().Lister(),
	}
}

func (s *PrivilegedUserSkipper) Skip(req admission.Request) (bool, error) {
	for _, pu := range s.PrivilegedUsers {
		if wildcard.Match(pu, req.UserInfo.Username) {
			return true, nil
		}
	}

	for _, pg := range s.PrivilegedGroups {
		for _, ug := range req.UserInfo.Groups {
			if wildcard.Match(pg, ug) {
				return true, nil
			}
		}
	}

	roles, clusterroles, err := userinfo.RoleRefs(s.RoleBindingLister, s.ClusterRoleBindingLister, req.UserInfo)
	if err != nil {
		return false, err
	}

	for _, pcr := range s.PrivilegedClusterRoles {
		for _, cr := range clusterroles {
			if wildcard.Match(pcr, cr) {
				return true, nil
			}
		}
	}

	for _, pr := range s.PrivilegedRoles {
		for _, r := range roles {
			if wildcard.Match(pr, r) {
				return true, nil
			}
		}
	}

	return false, nil
}
