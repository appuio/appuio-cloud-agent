package skipper

import (
	"context"

	"github.com/minio/pkg/wildcard"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper/userinfo"
)

var _ Skipper = &PrivilegedUserSkipper{}

// PrivilegedUserSkipper skips request validations for privileged users.
type PrivilegedUserSkipper struct {
	Client client.Reader

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

func (s *PrivilegedUserSkipper) Skip(ctx context.Context, req admission.Request) (bool, error) {
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

	clusterroles, err := userinfo.ClusterRoleRefsForUser(ctx, s.Client, req.UserInfo)
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
