package skipper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func Test_UserInfo_Skip(t *testing.T) {

	crs := []client.Object{
		&rbacv1.ClusterRoleBinding{
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
	}

	scheme := runtime.NewScheme()
	require.NoError(t, rbacv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crs...).Build()

	subject := PrivilegedUserSkipper{
		Client: c,

		PrivilegedGroups:       []string{"admins"},
		PrivilegedUsers:        []string{"chucktesta"},
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
			name: "not skipped user",
			userInfo: authenticationv1.UserInfo{
				Username: "not-skipped-user",
			},
			skipped: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			skipped, err := subject.Skip(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UserInfo: tc.userInfo,
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, tc.skipped, skipped)
		})
	}
}
