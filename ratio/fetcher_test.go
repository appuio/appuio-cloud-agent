package ratio

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	fooPod = podFromResources("foo1", "foo", podResource{
		containers: []containerResources{
			{cpu: "1", memory: "1Gi"},
			{cpu: "2", memory: "1Gi"},
		},
		phase: corev1.PodRunning,
	})
	foo2Pod = podFromResources("foo2", "foo", podResource{
		containers: []containerResources{{memory: "1Gi"}},
		phase:      corev1.PodRunning,
	})
	foobarPod = podFromResources("foo", "bar", podResource{
		containers: []containerResources{{memory: "1337Gi"}},
		phase:      corev1.PodRunning,
	})
)

func TestRatioValidator_Handle(t *testing.T) {
	ctx := context.Background()
	type testRatio map[string]struct {
		cpu    string
		memory string
	}
	tests := map[string]struct {
		namespace string
		objects   []client.Object
		orgLabel  string
		ratios    testRatio
		err       error
		errCheck  func(err error) bool
	}{
		"Fetch_EmptyNamespace": {
			namespace: "foo",
			ratios:    testRatio{},
		},
		"Fetch_Namespace": {
			namespace: "foo",
			objects: []client.Object{
				fooPod,
				foo2Pod,
				foobarPod,
			},
			ratios: testRatio{"": {
				memory: "3Gi",
				cpu:    "3",
			}},
		},
		"Fetch_WithDifferentNodeSelector": {
			namespace: "foo",
			objects: []client.Object{
				tap(fooPod, func(p *corev1.Pod) *corev1.Pod {
					p.Spec.NodeSelector = map[string]string{"class": "mega"}
					return p
				}),
				foo2Pod,
				tap(foo2Pod, func(p *corev1.Pod) *corev1.Pod {
					p.ObjectMeta.Name = "foo3"
					p.Spec.NodeSelector = map[string]string{"class": "huge"}
					return p
				}),
				foobarPod,
			},
			ratios: testRatio{
				"": {
					memory: "1Gi",
					cpu:    "0",
				},
				"class=huge": {
					memory: "1Gi",
					cpu:    "0",
				},
				"class=mega": {
					memory: "2Gi",
					cpu:    "3",
				},
			},
		},
		"Fetch_NotExists": {
			namespace: "not-exist",
			err:       errors.New("not-exist"),
			errCheck:  apierrors.IsNotFound,
		},
		"Fetch_Error": {
			namespace: "fail-foo",
			err:       errors.New("internal"),
			errCheck:  apierrors.IsInternalError,
		},
		"Fetch_OtherNamespace": {
			namespace: "bar",
			objects: []client.Object{
				fooPod,
				foo2Pod,
				foobarPod,
			},
			ratios: testRatio{"": {
				memory: "1337Gi",
				cpu:    "0",
			}},
		},
		"Fetch_WronglyDisabledNamespace": {
			namespace: "notdisabled-bar",
			objects: []client.Object{
				fooPod,
				foo2Pod,
				podFromResources("foo", "notdisabled-bar", podResource{
					containers: []containerResources{
						{memory: "1337Gi"},
					},
					phase: corev1.PodRunning,
				}),
			},
			ratios: testRatio{"": {
				memory: "1337Gi",
				cpu:    "0",
			}},
		},

		"Fetch_DisabledNamespace": {
			namespace: "disabled-bar",
			objects: []client.Object{
				fooPod,
				foo2Pod,
				podFromResources("foo", "disabled-bar", podResource{
					containers: []containerResources{
						{memory: "1337Gi"},
					},
					phase: corev1.PodRunning,
				}),
			},
			err: ErrorDisabled,
		},
		"Fetch_OtherDisabledNamespace": {
			namespace: "disabled-foo",
			objects: []client.Object{
				podFromResources("foo1", "disabled-foo", podResource{
					containers: []containerResources{
						{cpu: "1", memory: "1Gi"},
						{cpu: "2", memory: "1Gi"},
					},
					phase: corev1.PodRunning,
				}),
				foo2Pod,
				podFromResources("foo", "disabled-bar", podResource{
					containers: []containerResources{
						{memory: "1337Gi"},
					},
					phase: corev1.PodRunning,
				}),
			},
			err: ErrorDisabled,
		},
		"Fetch_NonOrgNamespace": {
			namespace: "foo",
			orgLabel:  "appuio.io/org",
			objects: []client.Object{
				fooPod,
				foo2Pod,
				podFromResources("foo", "disabled-bar", podResource{
					containers: []containerResources{
						{memory: "1337Gi"},
					},
					phase: corev1.PodRunning,
				}),
			},
			err: ErrorDisabled,
		},
		"Fetch_OrgNamespace": {
			namespace: "org",
			orgLabel:  "appuio.io/org",
			objects: []client.Object{
				podFromResources("foo1", "org", podResource{
					containers: []containerResources{
						{cpu: "1", memory: "1Gi"},
						{cpu: "2", memory: "1Gi"},
					},
					phase: corev1.PodRunning,
				}),
				podFromResources("foo2", "org", podResource{
					containers: []containerResources{
						{memory: "1Gi"},
					},
					phase: corev1.PodRunning,
				}),
				foobarPod,
			},
			ratios: testRatio{"": {
				memory: "3Gi",
				cpu:    "3",
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := prepareTest(t, testCfg{
				initObjs: tc.objects,
				orgLabel: tc.orgLabel,
			}).FetchRatios(ctx, tc.namespace)
			if tc.err == nil {
				require.NoError(t, err)
				require.Len(t, r, len(tc.ratios))
				for nodeSel, ratio := range tc.ratios {
					cpu := resource.MustParse(ratio.cpu)
					mem := resource.MustParse(ratio.memory)
					assert.Equal(t, *cpu.AsDec(), *r[nodeSel].CPU, "cpu requests should be equal for node selector %q", nodeSel)
					assert.Equal(t, *mem.AsDec(), *r[nodeSel].Memory, "memory requests should be equal for node selector %q", nodeSel)
				}
			} else {
				if tc.errCheck != nil {
					require.Truef(t, tc.errCheck(err), "Unexpected error")
				} else {
					require.ErrorIs(t, err, tc.err)
				}
			}
		})
	}
}

type testCfg struct {
	initObjs []client.Object
	orgLabel string
}

func prepareTest(t *testing.T, cfg testCfg) Fetcher {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	barNs := testNamespace("bar")
	barNs.Annotations = map[string]string{
		RatioValidatiorDisableAnnotation: "False",
	}
	orgNs := testNamespace("org")
	orgNs.Labels = map[string]string{
		cfg.orgLabel: "fooGmbh",
	}

	disabledNs := testNamespace("disabled-foo")
	disabledNs.Annotations = map[string]string{
		RatioValidatiorDisableAnnotation: "True",
	}
	otherDisabledNs := testNamespace("disabled-bar")
	otherDisabledNs.Annotations = map[string]string{
		RatioValidatiorDisableAnnotation: "true",
	}
	wronglyDisabledNs := testNamespace("notdisabled-bar")
	wronglyDisabledNs.Annotations = map[string]string{
		RatioValidatiorDisableAnnotation: "banana",
	}

	initObjs := append(cfg.initObjs, testNamespace("foo"), testNamespace("fail-foo"), orgNs, barNs, disabledNs, otherDisabledNs, wronglyDisabledNs)
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		Build()

	return Fetcher{
		Client:            failingClient{client},
		OrganizationLabel: cfg.orgLabel,
	}
}

// tap deep copies the given object and allows to modify it using the given function
func tap[T runtime.Object](t T, f func(T) T) T {
	ct := t.DeepCopyObject().(T)
	return f(ct)
}
