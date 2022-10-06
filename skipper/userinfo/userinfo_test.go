package userinfo_test

import (
	"testing"

	"github.com/appuio/appuio-cloud-agent/skipper/userinfo"
	"github.com/appuio/appuio-cloud-agent/skipper/userinfo/mocks"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRoleRefs(t *testing.T) {
	crLister := mocks.MockLister[*rbacv1.ClusterRoleBinding]{
		Objects: []*rbacv1.ClusterRoleBinding{
			{
				Subjects: []rbacv1.Subject{
					{
						Kind: "User",
						Name: "user-with-clusterrole",
					},
					{
						Kind: "Group",
						Name: "group-with-clusterrole",
					},
					{
						Kind:      "ServiceAccount",
						Name:      "sa-with-clusterrole",
						Namespace: "myns",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: "cluster-role",
				},
			},
		},
	}

	rLister := mocks.MockRoleBindingLister{
		MockLister: mocks.MockLister[*rbacv1.RoleBinding]{
			Objects: []*rbacv1.RoleBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "myns",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind: "ServiceAccount",
							Name: "default",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind: "Role",
						Name: "testrole",
					},
				}, {
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "myns",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind: "ServiceAccount",
							Name: "default",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind: "ClusterRole",
						Name: "referenced-from-role-binding",
					},
				},
			},
		},
	}

	testCases := []struct {
		name                 string
		userInfo             authenticationv1.UserInfo
		expectedClusterRoles []string
		expectedRoles        []string
	}{
		{
			name: "User ClusterRole",
			userInfo: authenticationv1.UserInfo{
				Username: "user-with-clusterrole",
			},
			expectedClusterRoles: []string{"cluster-role"},
		},
		{
			name: "Group ClusterRole",
			userInfo: authenticationv1.UserInfo{
				Groups: []string{"group-with-clusterrole"},
			},
			expectedClusterRoles: []string{"cluster-role"},
		},
		{
			name: "ServiceAccount ClusterRole",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:myns:sa-with-clusterrole",
			},
			expectedClusterRoles: []string{"cluster-role"},
		},
		{
			name: "User Role",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:myns:default",
			},
			expectedRoles:        []string{"myns:testrole"},
			expectedClusterRoles: []string{"myns:referenced-from-role-binding"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			roles, clusterRoles, err := userinfo.RoleRefs(&rLister, &crLister, tc.userInfo)
			require.NoError(t, err)
			require.ElementsMatch(t, tc.expectedClusterRoles, clusterRoles)
			require.ElementsMatch(t, tc.expectedRoles, roles)
		})
	}
}
