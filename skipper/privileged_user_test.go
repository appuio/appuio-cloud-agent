package skipper

import (
	"testing"

	"github.com/appuio/appuio-cloud-agent/skipper/userinfo/mocks"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func Test_UserInfo_Skip(t *testing.T) {

	crLister := mocks.MockLister[*rbacv1.ClusterRoleBinding]{
		Objects: []*rbacv1.ClusterRoleBinding{
			{
				Subjects: []rbacv1.Subject{
					{
						Kind: "User",
						Name: "user-with-clusterrole",
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: "cluster-image-registry-operator",
				},
			},
		},
	}

	rLister := mocks.MockRoleBindingLister{
		MockLister: mocks.MockLister[*rbacv1.RoleBinding]{
			Objects: []*rbacv1.RoleBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "argocd",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind: "ServiceAccount",
							Name: "default",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Kind: "Role",
						Name: "default",
					},
				},
			},
		},
	}

	subject := PrivilegedUserSkipper{
		ClusterRoleBindingLister: &crLister,
		RoleBindingLister:        &rLister,

		PrivilegedGroups:       []string{"admins"},
		PrivilegedUsers:        []string{"chucktesta"},
		PrivilegedRoles:        []string{"argocd:default"},
		PrivilegedClusterRoles: []string{"cluster-*-operator"},
	}

	testCases := []struct {
		name     string
		userInfo authenticationv1.UserInfo
		skipped  bool
	}{
		{
			name: "user in allowed group",
			userInfo: authenticationv1.UserInfo{
				Groups: []string{"admins"},
			},
			skipped: true,
		}, {
			name: "user in allowed user",
			userInfo: authenticationv1.UserInfo{
				Username: "chucktesta",
			},
			skipped: true,
		}, {
			name: "user with allowed ClusterRole",
			userInfo: authenticationv1.UserInfo{
				Username: "user-with-clusterrole",
			},
			skipped: true,
		}, {
			name: "user with allowed Role",
			userInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:argocd:default",
			},
			skipped: true,
		}, {
			name: "not skipped user",
			userInfo: authenticationv1.UserInfo{
				Username: "not-skipped-user",
			},
			skipped: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			skipped, err := subject.Skip(admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UserInfo: tc.userInfo,
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, tc.skipped, skipped)
		})
	}
}
