package webhooks

import (
	"context"
	"fmt"
	"testing"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/require"
	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

const testDefaultOrgAnnotation = "example.com/default-organization"

func Test_NamespaceProjectOrganizationMutator_Handle(t *testing.T) {
	const orgLabel = "example.com/organization"

	testCases := []struct {
		name string

		object client.Object

		additionalObjects func(t *testing.T) []client.Object

		user string

		allowed  bool
		orgPatch string
	}{
		{
			name: "Project: request with org label set",

			object: newProjectRequest("project", map[string]string{
				orgLabel: "some-org",
			}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "blub"),
					newGroup("some-org", "user"),
				}
			},

			user:    "user",
			allowed: true,
		},
		{
			name: "Namespace: request with org label set",

			object: newNamespace("project", map[string]string{
				orgLabel: "some-org",
			}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "blub"),
					newGroup("some-org", "user"),
				}
			},

			user:    "user",
			allowed: true,
		},

		{
			name: "Project: user with default org, no org set on object",

			object: newProjectRequest("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "some-org"),
					newGroup("some-org", "user"),
				}
			},

			user:     "user",
			allowed:  true,
			orgPatch: "some-org",
		},
		{
			name: "Namespace: user with default org, no org set on object",

			object: newNamespace("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "some-org"),
					newGroup("some-org", "user"),
				}
			},

			user:     "user",
			allowed:  true,
			orgPatch: "some-org",
		},

		{
			name: "Project: request with org label set, user not in org",

			object: newProjectRequest("project", map[string]string{orgLabel: "other-org"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", ""),
					newGroup("other-org"),
				}
			},

			user:    "user",
			allowed: false,
		},
		{
			name: "Namespace: request with org label set, user not in org",

			object: newNamespace("project", map[string]string{orgLabel: "other-org"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", ""),
					newGroup("other-org"),
				}
			},

			user:    "user",
			allowed: false,
		},

		{
			name: "Project: default org, user not in org",

			object: newProjectRequest("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "user-default-org"),
					newGroup("user-default-org"),
				}
			},

			user:    "user",
			allowed: false,
		},
		{
			name: "Namespace: default org, user not in org",

			object: newNamespace("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "user-default-org"),
					newGroup("user-default-org"),
				}
			},

			user:    "user",
			allowed: false,
		},

		{
			name: "Project: default org, user not in org",

			object: newProjectRequest("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "user-default-org"),
					newGroup("user-default-org"),
				}
			},

			user:    "user",
			allowed: false,
		},
		{
			name: "Namespace: default org, user not in org",

			object: newNamespace("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", "user-default-org"),
					newGroup("user-default-org"),
				}
			},

			user:    "user",
			allowed: false,
		},

		{
			name: "Project: request non-existing org",

			object: newProjectRequest("project", map[string]string{orgLabel: "non-existing"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", ""),
				}
			},

			user:    "user",
			allowed: false,
		},
		{
			name: "Namespace: request non-existing org",

			object: newNamespace("project", map[string]string{orgLabel: "non-existing"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", ""),
				}
			},

			user:    "user",
			allowed: false,
		},

		{
			name: "Project: no org set, no default org",

			object: newProjectRequest("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", ""),
				}
			},

			user:    "user",
			allowed: false,
		},
		{
			name: "Namespace: no org set, no default org",

			object: newNamespace("project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newUser("user", ""),
				}
			},

			user:    "user",
			allowed: false,
		},

		{
			name: "Project: request from service account",

			object: newProjectRequest("new-project", map[string]string{orgLabel: "some-org"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newNamespace("project", map[string]string{
						orgLabel: "some-org",
					}, nil),
					newServiceAccount("ns-creator", "project"),
				}
			},

			user:    "system:serviceaccount:project:ns-creator",
			allowed: false,
		},
		{
			name: "Namespace: request from service account",

			object: newNamespace("new-project", map[string]string{orgLabel: "some-org"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newNamespace("project", map[string]string{
						orgLabel: "some-org",
					}, nil),
					newServiceAccount("ns-creator", "project"),
				}
			},

			user:    "system:serviceaccount:project:ns-creator",
			allowed: true,
		},
		{
			name: "Namespace: request from service account, no label on object",

			object: newNamespace("new-project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newNamespace("project", map[string]string{
						orgLabel: "some-org",
					}, nil),
					newServiceAccount("ns-creator", "project"),
				}
			},

			user:     "system:serviceaccount:project:ns-creator",
			allowed:  true,
			orgPatch: "some-org",
		},
		{
			name: "Namespace: request from service account for other organization",

			object: newNamespace("new-project", map[string]string{
				orgLabel: "other-org",
			}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newNamespace("project", map[string]string{
						orgLabel: "some-org",
					}, nil),
					newServiceAccount("ns-creator", "project"),
				}
			},

			user:    "system:serviceaccount:project:ns-creator",
			allowed: false,
		},
		{
			name: "Namespace: request from service account in namespace without org label and no org label set on object",

			object: newNamespace("new-project", nil, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newNamespace("project", nil, nil),
					newServiceAccount("ns-creator", "project"),
				}
			},

			user:    "system:serviceaccount:project:ns-creator",
			allowed: false,
		},
		{
			name: "Namespace: request from service account in namespace without org label and org label set on object",

			object: newNamespace("new-project", map[string]string{orgLabel: "some-org"}, nil),
			additionalObjects: func(*testing.T) []client.Object {
				return []client.Object{
					newNamespace("project", nil, nil),
					newServiceAccount("ns-creator", "project"),
				}
			},

			user:    "system:serviceaccount:project:ns-creator",
			allowed: false,
		},

		{
			name: "Project: no openshift.io/requester annotation",

			object: newProjectRequest("new-project", map[string]string{orgLabel: "some-org"}, nil),

			user:    "",
			allowed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s (Expect allowed: %v)", tc.name, tc.allowed), func(t *testing.T) {
			t.Parallel()

			var initObjs []client.Object
			if tc.additionalObjects != nil {
				initObjs = tc.additionalObjects(t)
			}

			c, scheme, decoder := prepareClient(t, initObjs...)

			subject := NamespaceProjectOrganizationMutator{
				Decoder: decoder,
				Client:  c,
				Skipper: skipper.StaticSkipper{},

				OrganizationLabel:                 orgLabel,
				UserDefaultOrganizationAnnotation: testDefaultOrgAnnotation,
			}

			// OpenShift project requests are a special case, we can't trust the user info in the request but OpenShift adds the original requester as an annotation
			if p, ok := tc.object.(*projectv1.ProjectRequest); ok {
				if p.Annotations == nil {
					p.Annotations = make(map[string]string)
				}
				p.Annotations[OpenShiftProjectRequesterAnnotation] = tc.user
			}
			amr := admissionRequestForObject(t, tc.object, scheme)
			amr.UserInfo.Username = tc.user
			amr.UserInfo.Groups = []string{}
			resp := subject.Handle(context.Background(), amr)
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.Equal(t, tc.allowed, resp.Allowed)

			if tc.orgPatch != "" {
				requireOrgPatch(t, tc.orgPatch, resp.Patches)
			} else {
				require.Empty(t, resp.Patches)
			}
		})
	}
}

func requireOrgPatch(t *testing.T, org string, ps []jsonpatch.Operation) {
	require.Len(t, ps, 1)
	require.Equal(t, jsonpatch.NewOperation("add", "/metadata/labels/example.com~1organization", org), ps[0])
}

func newGroup(name string, users ...string) *userv1.Group {
	return &userv1.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Users: users,
	}
}

func newUser(name string, defaultOrg string) *userv1.User {
	u := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{},
		},
	}
	if defaultOrg != "" {
		u.Annotations[testDefaultOrgAnnotation] = defaultOrg
	}
	return u
}

func newServiceAccount(name, namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
