package skipper

import (
	"github.com/minio/pkg/wildcard"
	kubeinformers "k8s.io/client-go/informers"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper/userinfo"
)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch

var _ Skipper = &PrivilegedUserSkipper{}

// PrivilegedUserSkipper skips request validations for privileged users.
type PrivilegedUserSkipper struct {
	ClusterRoleBindingLister rbacv1listers.ClusterRoleBindingLister

	PrivilegedGroups []string
	PrivilegedUsers  []string
	// PrivilegedClusterRoles is a list cluster roles allowed to bypass restrictions.
	// Wildcards are supported (e.g. "system:serviceaccount:default:*" or "cluster-*-operator").
	// ClusterRoles are only ever matched if they are bound through a ClusterRoleBinding,
	// this is different from the behavior of Kyverno.
	// This is done to prevent a user from wrongly configuring a low-privileged ClusterRole which users
	// can then bind to themselves to bypass the restrictions.
	PrivilegedClusterRoles []string
}

// NewPrivilegedUserSkipper creates a new PrivilegedUserSkipper with the *Lister set.
func NewPrivilegedUserSkipper(inf kubeinformers.SharedInformerFactory) *PrivilegedUserSkipper {
	return &PrivilegedUserSkipper{
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

	clusterroles, err := userinfo.ClusterRoleRefs(s.ClusterRoleBindingLister, req.UserInfo)
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

	return false, nil
}
