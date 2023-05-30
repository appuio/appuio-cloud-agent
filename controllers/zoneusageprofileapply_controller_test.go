package controllers

import (
	"context"
	"regexp"
	"testing"
	"time"

	controlv1 "github.com/appuio/control-api/apis/v1"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/controllers/transformers"
)

// Test_UsageProfileApplyReconciler_Reconcile tests the Reconcile function of the UsageProfileApplyReconciler.
func Test_ZoneUsageProfileApplyReconciler_Reconcile(t *testing.T) {
	orgLbl := "test.com/organization"

	// We need to bring the big guns to test the dynamic watches; envtest it is.
	// Test env must be stopped AFTER the manager has been stopped.
	// Otherwise the manager will block the shutdown by initiating watch requests to the API server. defer is LIFO.
	cfg, stop := setupEnvtestEnv(t)
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := testr.NewWithOptions(t, testr.Options{Verbosity: 9001})
	ctx = log.IntoContext(ctx, l)
	log.SetLogger(l)

	_, scheme, recorder := prepareClient(t)
	mgr, err := manager.New(cfg, manager.Options{Scheme: scheme})
	require.NoError(t, err)
	c := mgr.GetClient()

	sysNS := newNamespace("some-system-ns", nil, nil)
	require.NoError(t, c.Create(context.Background(), sysNS))
	org1NS := newNamespace("org1", map[string]string{orgLbl: "foo"}, nil)
	require.NoError(t, c.Create(context.Background(), org1NS))
	org2NS := newNamespace("org2", map[string]string{orgLbl: "bar"}, nil)
	require.NoError(t, c.Create(context.Background(), org2NS))

	subject := &ZoneUsageProfileApplyReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: recorder,
		Cache:    mgr.GetCache(),

		OrganizationLabel: orgLbl,

		Transformers: []transformers.Transformer{
			addTestAnnotationTransformer{},
		},
	}
	require.NoError(t, subject.SetupWithManager(mgr))
	go func() {
		defer cancel()
		require.NoError(t, mgr.Start(ctx))
	}()

	profile := buildUsageProfile(t, scheme, "test")
	require.NoError(t, c.Create(context.Background(), profile))

	// Check that the ResourceQuota was created in the correct namespace
	quota := &corev1.ResourceQuota{}
	requireEventually(t, func(collect *assert.CollectT) {
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: org1NS.Name}, quota))
	}, "should have created a ResourceQuota in the correct namespace")

	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: org1NS.Name}, quota))
	require.Equal(t, "666", quota.Spec.Hard.Cpu().String(), "should have applied manifest content")
	require.Equal(t, org1NS.Name, quota.Annotations["test"], "should have applied the configured transformer")
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: org2NS.Name}, quota))
	require.Error(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: sysNS.Name}, quota))

	// Test new org namespace
	newOrgNS := newNamespace("org3", map[string]string{orgLbl: "baz"}, nil)
	require.NoError(t, c.Create(context.Background(), newOrgNS))
	requireEventually(t, func(collect *assert.CollectT) {
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: newOrgNS.Name}, quota))
	}, "should have created a ResourceQuota in newly created namespace")

	// Test dynamic watches
	// The controller dynamically watches resources it creates and keeps them in sync.
	quota.Spec.Hard = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("777"),
	}
	require.NoError(t, c.Update(context.Background(), quota))

	requireEventually(t, func(collect *assert.CollectT) {
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "org-usage", Namespace: org1NS.Name}, quota))
		require.Equal(t, "666", quota.Spec.Hard.Cpu().String(), "should have reset manifest content")
	}, "should have updated the ResourceQuota in the correct namespace")

	// Test errors on conflicting profiles
	conflictingProfile := buildUsageProfile(t, scheme, "conflict")
	require.NoError(t, c.Create(context.Background(), conflictingProfile))

	requireEventually(t, func(collect *assert.CollectT) {
		require.GreaterOrEqual(t, len(recorder.Events), 2, "should have recorded two events")
	}, "should have created a ResourceQuota in the correct namespace")
	assert.Contains(t, <-recorder.Events, "conflict", regexp.MustCompile(`^Warning.*conflict`))
	assert.Contains(t, <-recorder.Events, "conflict", regexp.MustCompile(`^Warning.*conflict`))
}

func requireEventually(t *testing.T, f func(collect *assert.CollectT), msgAndArgs ...interface{}) {
	t.Helper()
	require.EventuallyWithT(t, f, 10*time.Second, time.Second/10, msgAndArgs...)
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
									corev1.ResourceCPU: resource.MustParse("666"),
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
