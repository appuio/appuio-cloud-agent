package controllers

import (
	"context"
	"regexp"
	"testing"

	controlv1 "github.com/appuio/control-api/apis/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/controllers/transformers"
)

// Test_UsageProfileApplyReconciler_Reconcile tests the Reconcile function of the UsageProfileApplyReconciler.
func Test_ZoneUsageProfileApplyReconciler_Reconcile(t *testing.T) {
	orgLbl := "test.com/organization"

	sysNS := newNamespace("kube-system", nil, nil)
	org1NS := newNamespace("org1", map[string]string{orgLbl: "foo"}, nil)
	org2NS := newNamespace("org2", map[string]string{orgLbl: "bar"}, nil)
	c, scheme, recorder := prepareClient(t, sysNS, org1NS, org2NS)

	profile := buildUsageProfile(t, scheme, "test")
	require.NoError(t, c.Create(context.Background(), profile))

	subject := &ZoneUsageProfileApplyReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: recorder,

		OrganizationLabel: orgLbl,

		Transformers: []transformers.Transformer{
			addTestAnnotationTransformer{},
		},
	}
	_, err := subject.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: profile.Name}})
	require.NoError(t, err)

	// Check that the ResourceQuota was created in the correct namespace
	quota := &corev1.ResourceQuota{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: org1NS.Name}, quota))
	require.Equal(t, "666", quota.Spec.Hard.Cpu().String(), "should have applied manifest content")
	require.Equal(t, org1NS.Name, quota.Annotations["test"], "should have applied the configured transformer")
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: org2NS.Name}, quota))
	require.Error(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: sysNS.Name}, quota))

	// Test errors on conflicting profiles
	conflictingProfile := buildUsageProfile(t, scheme, "conflict")
	require.NoError(t, c.Create(context.Background(), conflictingProfile))

	_, err = subject.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: conflictingProfile.Name}})
	require.ErrorContains(t, err, "conflict")
	require.Len(t, recorder.Events, 2, "should have recorded two events")
	assert.Contains(t, <-recorder.Events, "conflict", regexp.MustCompile(`^Warning.*conflict`))
	assert.Contains(t, <-recorder.Events, "conflict", regexp.MustCompile(`^Warning.*conflict`))
}

func Test_labelExistsPredicate(t *testing.T) {
	lbl := "test.com/organization"
	subject, err := labelExistsPredicate(lbl)
	require.NoError(t, err)

	assert.True(t, subject.Generic(event.GenericEvent{
		Object: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{lbl: "foo"},
			},
		},
	}))
	assert.False(t, subject.Generic(event.GenericEvent{
		Object: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"other": "foo"},
			},
		},
	}))
}

func Test_mapToAllUsageProfiles(t *testing.T) {
	c, _, _ := prepareClient(t,
		&cloudagentv1.ZoneUsageProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test1",
			},
		},
		&cloudagentv1.ZoneUsageProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2",
			},
		},
	)

	subject := mapToAllUsageProfiles(c)
	assert.ElementsMatch(t,
		[]reconcile.Request{
			{NamespacedName: types.NamespacedName{Name: "test1"}},
			{NamespacedName: types.NamespacedName{Name: "test2"}},
		},
		subject(context.Background(), &corev1.Namespace{}),
		"should map any event to all UsageProfiles",
	)
}

// Test_Conversions tests the conversion between runtime.RawExtension and unstructured.Unstructured for ensuring compatibility when upgrading packages.
func Test_Conversions(t *testing.T) {
	_, scheme, _ := prepareClient(t)

	// Does not work
	assert.Error(t, scheme.Convert(&runtime.RawExtension{}, &unstructured.Unstructured{}, nil))

	// Does work
	r := &runtime.RawExtension{
		Object: &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					"cpu": resource.MustParse("1"),
				},
			},
		},
	}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(r)
	assert.NoError(t, err)
	u := &unstructured.Unstructured{Object: raw}
	var _ client.Object = u
	j, err := u.MarshalJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, `{"metadata":{"creationTimestamp":null,"name":"test","namespace":"test"},"spec":{"hard":{"cpu":"1"}},"status":{}}`, string(j))

	// Sanity check
	assert.NoError(t, scheme.Convert(&corev1.Pod{}, &unstructured.Unstructured{}, nil))
}

// buildUsageProfile builds a valid ZoneUsageProfile with a ResourceQuota named test.
func buildUsageProfile(t *testing.T, scheme *runtime.Scheme, name string) *cloudagentv1.ZoneUsageProfile {
	t.Helper()

	return &cloudagentv1.ZoneUsageProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: cloudagentv1.ZoneUsageProfileSpec{
			UpstreamSpec: controlv1.UsageProfileSpec{
				Resources: map[string]runtime.RawExtension{
					"org-usage": {
						Object: ensureGVK(t, scheme, &corev1.ResourceQuota{
							ObjectMeta: metav1.ObjectMeta{Name: "test"},
							Spec: corev1.ResourceQuotaSpec{
								Hard: corev1.ResourceList{
									"cpu": resource.MustParse("666"),
								},
							},
						}),
					},
				},
			},
		},
	}
}

// take the name from the namespace and adds it as an annotation
type addTestAnnotationTransformer struct{}

func (a addTestAnnotationTransformer) Transform(ctx context.Context, obj *unstructured.Unstructured, ns *corev1.Namespace) error {
	obj.SetAnnotations(map[string]string{"test": ns.Name})
	return nil
}
