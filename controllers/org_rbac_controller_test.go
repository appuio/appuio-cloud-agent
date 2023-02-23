package controllers

import (
	"context"
	"errors"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestOrganizationRBACReconciler(t *testing.T) {

	orgLabel := "appuio.io/organization"
	defaultCRs := map[string]string{
		"admin": "admin",
	}

	type ns struct {
		name   string
		labels map[string]string
	}
	type rb struct {
		name   string
		labels map[string]string

		roleRef string
		groups  []string
	}

	tcs := map[string]struct {
		clusterRoles map[string]string

		fail   bool
		events int

		namespace string
		nsLabels  map[string]string

		roleBindings []rb
		expected     []rb
	}{
		"NonLabelNs_Noop": {
			clusterRoles: defaultCRs,
			namespace:    "foo",
		},
		"NonOrgNs_Noop": {
			clusterRoles: defaultCRs,
			namespace:    "bar",
			nsLabels: map[string]string{
				"appuio.io/noorganization": "vshn",
			},
		},
		"InvalidOrgNs_Noop": {
			clusterRoles: defaultCRs,
			namespace:    "buzz",
			nsLabels: map[string]string{
				orgLabel: "",
			},
		},
		"NoAccessOrgNs": {
			clusterRoles: defaultCRs,
			namespace:    "buzz",
			nsLabels: map[string]string{
				"appuio.io/no-rbac-creation": "true",
				orgLabel:                     "foo",
			},
		},
		"NoRbacCreationFalseOrgNs_CreateRole": {
			clusterRoles: defaultCRs,
			namespace:    "buzz",
			nsLabels: map[string]string{
				"appuio.io/no-rbac-creation": "false",
				orgLabel:                     "foo",
			},

			expected: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"foo"},
				},
			},
		},

		"OrgNs_CreateRole": {
			clusterRoles: defaultCRs,
			namespace:    "buzz",
			nsLabels: map[string]string{
				orgLabel: "foo",
			},

			expected: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"foo"},
				},
			},
		},
		"OrgNs_CreateMultipleRoles": {
			clusterRoles: map[string]string{
				"foo": "admin",
				"bar": "bar",
			},
			namespace: "buzz",
			nsLabels: map[string]string{
				orgLabel: "foo",
			},

			expected: []rb{
				{
					name:    "foo",
					roleRef: "admin",
					groups:  []string{"foo"},
				},
				{
					name:    "bar",
					roleRef: "bar",
					groups:  []string{"foo"},
				},
			},
		},

		"OrgNs_KeepRole": {
			clusterRoles: defaultCRs,
			namespace:    "buzz",
			nsLabels: map[string]string{
				orgLabel: "foo",
			},

			roleBindings: []rb{
				{
					name:    "admin",
					roleRef: "old-admin",
					groups:  []string{"buzz", "tom"},
				},
			},
			expected: []rb{
				{
					name:    "admin",
					roleRef: "old-admin",
					groups:  []string{"buzz", "tom"},
				},
			},
		},
		"OrgNs_UpdateUninitialized": {
			clusterRoles: defaultCRs,
			namespace:    "uninit",
			nsLabels: map[string]string{
				orgLabel: "foo",
			},

			roleBindings: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"buzz", "tom"},
					labels: map[string]string{
						LabelRoleBindingUninitialized: "true",
					},
				},
			},
			expected: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"foo"},
				},
			},
		},
		"OrgNs_CreateUpdateAndKeep": {
			clusterRoles: map[string]string{
				"admin": "admin",
				"keep":  "bar",
				"new":   "adminv2",
			},
			namespace: "complex",
			nsLabels: map[string]string{
				orgLabel: "foo",
			},

			roleBindings: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"buzz", "tom"},
					labels: map[string]string{
						LabelRoleBindingUninitialized: "true",
					},
				},
				{
					name:    "keep",
					roleRef: "bar",
					groups:  []string{"buzz", "tom", "bob"},
				},
			},
			expected: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"foo"},
				},
				{
					name:    "keep",
					roleRef: "bar",
					groups:  []string{"buzz", "tom", "bob"},
				},
				{
					name:    "new",
					roleRef: "adminv2",
					groups:  []string{"foo"},
				},
			},
		},
		"OrgNs_CreateUpdateKeepAndFail": {
			clusterRoles: map[string]string{
				"fail-admib": "fail",
				"admin":      "admin",
				"keep":       "bar",
				"new":        "adminv2",
			},
			namespace: "complex",
			nsLabels: map[string]string{
				orgLabel: "foo",
			},

			fail:   true,
			events: 1,

			roleBindings: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"buzz", "tom"},
					labels: map[string]string{
						LabelRoleBindingUninitialized: "true",
					},
				},
				{
					name:    "keep",
					roleRef: "bar",
					groups:  []string{"buzz", "tom", "bob"},
				},
			},
			expected: []rb{
				{
					name:    "admin",
					roleRef: "admin",
					groups:  []string{"foo"},
				},
				{
					name:    "keep",
					roleRef: "bar",
					groups:  []string{"buzz", "tom", "bob"},
				},
				{
					name:    "new",
					roleRef: "adminv2",
					groups:  []string{"foo"},
				},
			},
		},
	}

	for name, tc := range tcs {

		obj := []client.Object{}
		obj = append(obj, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   tc.namespace,
				Labels: tc.nsLabels,
			},
		})

		for _, rb := range tc.roleBindings {
			subs := []rbacv1.Subject{}
			for _, sub := range rb.groups {
				subs = append(subs, rbacv1.Subject{
					Kind:     rbacv1.GroupKind,
					APIGroup: rbacv1.GroupName,
					Name:     sub,
				})
			}
			obj = append(obj, &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rb.name,
					Namespace: tc.namespace,
					Labels:    rb.labels,
				},
				Subjects: subs,
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					APIGroup: rbacv1.GroupName,
					Name:     rb.roleRef,
				},
			})
		}

		t.Run(name, func(t *testing.T) {
			recorder := record.NewFakeRecorder(4)
			r := prepareOranizationRBACTest(t, testOrganizationRBACfg{
				obj:               obj,
				recorder:          recorder,
				organizationLabel: orgLabel,
				clusterRoles:      tc.clusterRoles,
			})

			ctx := log.IntoContext(context.TODO(), log.Log.WithName("debug"))
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: tc.namespace,
				},
			})
			if tc.fail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			var foundRBs rbacv1.RoleBindingList
			err = r.Client.List(context.TODO(), &foundRBs, client.InNamespace(tc.namespace))
			require.NoError(t, err)

			assert.Len(t, foundRBs.Items, len(tc.expected), "Unexpected number of roleBindings")
			assert.Len(t, recorder.Events, tc.events, "Unexpected number of events emitted")

			for _, expected := range tc.expected {
				present := false
				for _, found := range foundRBs.Items {
					if found.Name == expected.name {
						present = true

						assert.Equal(t, "ClusterRole", found.RoleRef.Kind)
						assert.Equal(t, expected.roleRef, found.RoleRef.Name)

						var foundGroups []string
						for _, sub := range found.Subjects {
							foundGroups = append(foundGroups, sub.Name)
						}
						assert.ElementsMatch(t, expected.groups, foundGroups)

						assert.False(t, rolebindingIsUninitialized(&found), "roleBinding should be marked as initialized")

						require.Len(t, found.OwnerReferences, 1)
						assert.Equal(t, tc.namespace, found.OwnerReferences[0].Name)
						break
					}
				}
				assert.Truef(t, present, "missing roleBinding %q", expected.name)
			}
		})
	}
}

type testOrganizationRBACfg struct {
	obj               []client.Object
	recorder          record.EventRecorder
	organizationLabel string
	clusterRoles      map[string]string
}

func prepareOranizationRBACTest(t *testing.T, cfg testOrganizationRBACfg) *OrganizationRBACReconciler {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	client := failingClient{
		fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cfg.obj...).
			Build(),
	}

	if cfg.recorder == nil {
		cfg.recorder = &record.FakeRecorder{}
	}

	return &OrganizationRBACReconciler{
		Client:              client,
		Recorder:            cfg.recorder,
		Scheme:              scheme,
		OrganizationLabel:   cfg.organizationLabel,
		DefaultClusterRoles: cfg.clusterRoles,
	}
}

type failingClient struct {
	client.WithWatch
}

func (c failingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if strings.HasPrefix(obj.GetName(), "fail-") {
		return apierrors.NewInternalError(errors.New("ups"))
	}
	return c.WithWatch.Create(ctx, obj, opts...)
}
func (c failingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if strings.HasPrefix(obj.GetName(), "fail-") {
		return apierrors.NewInternalError(errors.New("ups"))
	}
	return c.WithWatch.Update(ctx, obj, opts...)
}
