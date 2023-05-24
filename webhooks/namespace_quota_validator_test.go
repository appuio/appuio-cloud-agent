package webhooks

import (
	"context"

	"github.com/appuio/appuio-cloud-agent/skipper"
	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"testing"
)

func TestNamespaceQuotaValidator_Handle(t *testing.T) {
	ctx := context.Background()
	const orgLabel = "test.io/organization"
	const userDefaultOrgAnnotation = "test.io/default-organization"
	const nsLimit = 2

	tests := map[string]struct {
		initObjects []client.Object
		object      client.Object
		allowed     bool
		matchReason string
	}{
		"Allow Namespace": {
			initObjects: []client.Object{
				newNamespace("a", map[string]string{orgLabel: "other"}, nil), newNamespace("b", map[string]string{orgLabel: "other"}, nil),
				newNamespace("an", nil, nil), newNamespace("bn", nil, nil),
			},
			object: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						orgLabel: "testorg",
					},
				},
			},
			allowed: true,
		},
		"Allow ProjectRequest": {
			initObjects: []client.Object{
				newNamespace("a", map[string]string{orgLabel: "other"}, nil), newNamespace("b", map[string]string{orgLabel: "other"}, nil),
				newNamespace("an", nil, nil), newNamespace("bn", nil, nil),
			},
			object: &projectv1.ProjectRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						orgLabel: "testorg",
					},
				},
			},
			allowed: true,
		},

		"Deny Namespace TooMany": {
			initObjects: []client.Object{newNamespace("a", map[string]string{orgLabel: "testorg"}, nil), newNamespace("b", map[string]string{orgLabel: "testorg"}, nil)},
			object: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						orgLabel: "testorg",
					},
				},
			},
			allowed:     false,
			matchReason: "You cannot create more than 2 namespaces for organization \"testorg\"",
		},
		"Deny ProjectRequest TooMany": {
			initObjects: []client.Object{newNamespace("a", map[string]string{orgLabel: "testorg"}, nil), newNamespace("b", map[string]string{orgLabel: "testorg"}, nil)},
			object: &projectv1.ProjectRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						orgLabel: "testorg",
					},
				},
			},
			allowed:     false,
			matchReason: "You cannot create more than 2 namespaces for organization \"testorg\"",
		},

		"Deny Namespace TooMany GetOrganizationFromUser": {
			initObjects: []client.Object{
				newNamespace("a", map[string]string{orgLabel: "testorg"}, nil),
				newNamespace("b", map[string]string{orgLabel: "testorg"}, nil),
				&userv1.User{
					ObjectMeta: metav1.ObjectMeta{
						Name: "user",
						Annotations: map[string]string{
							userDefaultOrgAnnotation: "testorg",
						},
					},
				},
			},
			object: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			allowed: false,
		},

		"Deny NoOrganization": {
			initObjects: []client.Object{
				&userv1.User{
					ObjectMeta: metav1.ObjectMeta{
						Name: "user",
					},
				},
			},
			object: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			allowed:     false,
			matchReason: "There is no organization label and the user has no default organization set",
		},

		"Deny NoOrganizationLabelAndNoUser": {
			object: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			allowed: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			c, _, dec := prepareClient(t, test.initObjects...)
			subject := &NamespaceQuotaValidator{
				Client:  c,
				Skipper: skipper.StaticSkipper{ShouldSkip: false},

				OrganizationLabel:                 orgLabel,
				UserDefaultOrganizationAnnotation: userDefaultOrgAnnotation,
				DefaultNamespaceCountLimit:        nsLimit,
			}
			subject.InjectDecoder(dec)

			res := subject.Handle(ctx, admissionRequestForObject(t, test.object))
			require.Equal(t, test.allowed, res.Allowed)
			if test.matchReason != "" {
				require.Contains(t, res.Result.Reason, test.matchReason)
			}
		})
	}
}
