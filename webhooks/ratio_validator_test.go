package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"testing"

	"github.com/appuio/appuio-cloud-agent/limits"
	"github.com/appuio/appuio-cloud-agent/ratio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRatioValidator_Handle(t *testing.T) {
	ctx := context.Background()
	tests := map[string]struct {
		user         string
		namespace    string
		resources    []client.Object
		object       client.Object
		mangleObject bool
		create       bool
		limits       limits.Limits
		warn         bool
		fail         bool
		statusCode   int32
	}{

		"Allow_EmptyNamespace": {
			user:      "appuio#foo",
			namespace: "foo",
			limits:    limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:      false,
		},
		"Allow_FairNamespace": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "100m", memory: "3Gi"},
				}),
				podFromResources("pod2", "foo", podResource{
					{cpu: "50m", memory: "1Gi"},
				}),
				podFromResources("unfair", "bar", podResource{
					{cpu: "5", memory: "1Gi"},
				}),
			},
			limits: limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:   false,
		},
		"Warn_UnfairNamespace": {
			user:      "appuio#foo",
			namespace: "bar",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "100m", memory: "3G"},
				}),
				podFromResources("pod2", "foo", podResource{
					{cpu: "50m", memory: "1Gi"},
				}),
				podFromResources("unfair", "bar", podResource{
					{cpu: "8", memory: "1Gi"},
				}),
			},
			limits: limits.Limits{{Limit: requireParseQuantity(t, "1Gi")}},
			warn:   true,
		},
		"Warn_UnfairNamespace_Selectors": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{
				podFromResources("fair", "foo", podResource{
					{cpu: "1", memory: "4Gi"},
				}, func(p *corev1.Pod) {
					p.Spec.NodeSelector = map[string]string{"class": "foo"}
				}),
				podFromResources("unfair", "foo", podResource{
					{cpu: "2", memory: "1Gi"},
				}, func(p *corev1.Pod) {
					p.Spec.NodeSelector = map[string]string{"class": "bar"}
				}),
			},
			limits: limits.Limits{{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"class": "bar"},
				},
				Limit: requireParseQuantity(t, "1Gi"),
			}},
			warn: true,
		},
		"Allow_DisabledUnfairNamespace": {
			user:      "appuio#foo",
			namespace: "disabled-foo",
			resources: []client.Object{
				podFromResources("unfair", "disabled-foo", podResource{
					{cpu: "8", memory: "1Gi"},
				}),
			},
			limits: limits.Limits{{Limit: requireParseQuantity(t, "1Gi")}},
			warn:   false,
		},
		"Allow_LowercaseDisabledUnfairNamespace": {
			user:      "appuio#foo",
			namespace: "disabled-bar",
			resources: []client.Object{
				podFromResources("unfair", "disabled-bar", podResource{
					{cpu: "8", memory: "1Gi"},
				}),
			},
			limits: limits.Limits{{Limit: requireParseQuantity(t, "1Gi")}},
			warn:   false,
		},

		"Allow_ServiceAccount": {
			user:      "system:serviceaccount:bar",
			namespace: "bar",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "100m", memory: "3G"},
				}),
				podFromResources("pod2", "foo", podResource{
					{cpu: "50m", memory: "1Gi"},
				}),
				podFromResources("unfair", "bar", podResource{
					{cpu: "8", memory: "1Gi"},
				}),
			},
			limits: limits.Limits{{Limit: requireParseQuantity(t, "1Gi")}},
			warn:   false,
		},
		"ListFailure": {
			user:      "bar",
			namespace: "fail-bar",
			resources: []client.Object{
				testNamespace("fail-bar"),
				podFromResources("pod1", "foo", podResource{
					{cpu: "100m", memory: "3G"},
				}),
				podFromResources("pod2", "foo", podResource{
					{cpu: "50m", memory: "1Gi"},
				}),
				podFromResources("unfair", "fail-bar", podResource{
					{cpu: "8", memory: "1Gi"},
				}),
			},
			limits:     limits.Limits{{Limit: requireParseQuantity(t, "1Gi")}},
			warn:       false,
			fail:       true,
			statusCode: http.StatusInternalServerError,
		},
		"NamespaceNotExists": {
			user:       "bar",
			namespace:  "notexits",
			resources:  []client.Object{},
			limits:     limits.Limits{{Limit: requireParseQuantity(t, "1Gi")}},
			warn:       false,
			fail:       true,
			statusCode: http.StatusNotFound,
		},

		"Warn_ConsiderNewPod": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "100m", memory: "4Gi"},
				}),
				podFromResources("pod2", "foo", podResource{
					{cpu: "50m", memory: "4Gi"},
				}),
			},
			object: podFromResources("unfair", "foo", podResource{
				{cpu: "8", memory: "1Gi"},
			}),
			limits: limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:   true,
			create: true,
		},
		"Warn_FailMangledPod": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{},
			object: podFromResources("unfair", "foo", podResource{
				{cpu: "8", memory: "1Gi"},
			}),
			mangleObject: true,
			limits:       limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:         false,
			create:       true,
			fail:         true,
			statusCode:   http.StatusBadRequest,
		},
		"Warn_ConsiderNewDeployment_EmptyNS": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{},
			object: deploymentFromResources("unfair", "foo", 2, podResource{
				{cpu: "2", memory: "1Gi"},
			}),
			limits: limits.Limits{{
				Limit: requireParseQuantity(t, "1Gi"),
			}},
			warn:   true,
			create: true,
		},
		"Warn_ConsiderNewDeployment_NodeSelectorFromNamespace": {
			user:      "appuio#foo",
			namespace: "ns-with-default-node-selector",
			resources: []client.Object{},
			object: deploymentFromResources("ns-with-default-node-selector", "foo", 2, podResource{
				{cpu: "2", memory: "1Gi"},
			}),
			limits: limits.Limits{{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"class": "plus"},
				},
				Limit: requireParseQuantity(t, "1Gi"),
			}},
			warn:   true,
			create: true,
		},
		"Warn_ConsiderNewDeployment_FuzzyMatchNodeSelector": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "1", memory: "1Gi"},
				}, func(p *corev1.Pod) {
					p.Spec.NodeSelector = map[string]string{"app": "true", "class": "foo"}
				}),
			},
			object: deploymentFromResources("unfair", "foo", 2, podResource{
				{cpu: "2", memory: "1Gi"},
			}, func(d *appsv1.Deployment) {
				d.Spec.Template.Spec.NodeSelector = map[string]string{"class": "foo"}
			}),
			limits: limits.Limits{{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "true"},
				},
				Limit: requireParseQuantity(t, "1Gi"),
			}},
			warn:   true,
			create: true,
		},
		"Warn_FailMangledDeployment": {
			user:         "appuio#foo",
			namespace:    "foo",
			resources:    []client.Object{},
			object:       deploymentFromResources("unfair", "foo", 2, podResource{}),
			mangleObject: true,
			limits:       limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:         false,
			create:       true,
			fail:         true,
			statusCode:   http.StatusBadRequest,
		},
		"Warn_ConsiderNewStatefulset": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "0", memory: "4Gi"},
				}),
			},
			object: statefulsetFromResources("unfair", "foo", 2, podResource{
				{cpu: "1", memory: "1Gi"},
			}),
			limits: limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:   true,
			create: true,
		},
		"Warn_FailMangledSts": {
			user:         "appuio#foo",
			namespace:    "foo",
			resources:    []client.Object{},
			object:       statefulsetFromResources("unfair", "foo", 2, podResource{}),
			mangleObject: true,
			limits:       limits.Limits{{Limit: requireParseQuantity(t, "4Gi")}},
			warn:         false,
			create:       true,
			fail:         true,
			statusCode:   http.StatusBadRequest,
		},
	}

	for name, tc := range tests {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := prepareTest(t, tc.resources...)
			v.RatioLimits = tc.limits

			ar := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "e515f52d-7181-494d-a3d3-f0738856bd97",
					Kind: metav1.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "ConfigMap",
					},
					Name:      "test",
					Namespace: tc.namespace,
					Operation: admissionv1.Update,
					UserInfo: authenticationv1.UserInfo{
						Username: tc.user,
						Groups: []string{
							"oidc:user",
						},
					},
				},
			}
			if tc.object != nil {
				kind := tc.object.GetObjectKind().GroupVersionKind()
				ar.AdmissionRequest.Kind.Group = kind.Group
				ar.AdmissionRequest.Kind.Version = kind.Version
				ar.AdmissionRequest.Kind.Kind = kind.Kind

				ar.AdmissionRequest.Name = tc.object.GetName()

				raw, err := json.Marshal(tc.object)
				require.NoError(t, err)
				if tc.mangleObject {
					raw = []byte("?invalid")
				}

				ar.AdmissionRequest.Object = runtime.RawExtension{
					Raw: raw,
				}

			}
			if tc.create {
				ar.AdmissionRequest.Operation = admissionv1.Create
			}

			resp := v.Handle(ctx, ar)
			if tc.fail {
				require.NotNil(t, resp.AdmissionResponse.Result)
				assert.Equal(t, tc.statusCode, resp.AdmissionResponse.Result.Code)
				assert.True(t, resp.Allowed)
				return
			}
			if resp.AdmissionResponse.Result != nil {
				assert.EqualValues(t, http.StatusOK, resp.AdmissionResponse.Result.Code)
			}
			assert.True(t, resp.Allowed)
			if tc.warn {
				assert.NotEmpty(t, resp.AdmissionResponse.Warnings)
				for _, w := range resp.AdmissionResponse.Warnings {
					assert.NotContainsf(t, w, "\n", "Warning are not allowed to contain newlines")
				}
			} else {
				assert.Empty(t, resp.AdmissionResponse.Warnings)
			}
		})
	}
}

func prepareTest(t *testing.T, initObjs ...client.Object) *RatioValidator {
	const defaultNodeSelectorAnnotation = "test.io/node-selector"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	decoder, err := admission.NewDecoder(scheme)
	require.NoError(t, err)
	barNs := testNamespace("bar")
	barNs.Annotations = map[string]string{
		ratio.RatioValidatiorDisableAnnotation: "False",
	}

	disabledNs := testNamespace("disabled-foo")
	disabledNs.Annotations = map[string]string{
		ratio.RatioValidatiorDisableAnnotation: "True",
	}
	otherDisabledNs := testNamespace("disabled-bar")
	otherDisabledNs.Annotations = map[string]string{
		ratio.RatioValidatiorDisableAnnotation: "true",
	}

	nsWithDefaultNodeSelector := testNamespace("ns-with-default-node-selector")
	nsWithDefaultNodeSelector.Annotations = map[string]string{
		defaultNodeSelectorAnnotation: "class=plus",
	}

	initObjs = append(initObjs, testNamespace("foo"), barNs, disabledNs, otherDisabledNs, nsWithDefaultNodeSelector)
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		Build()

	uv := &RatioValidator{
		DefaultNamespaceNodeSelectorAnnotation: defaultNodeSelectorAnnotation,

		Client: failingClient{client},

		Ratio: ratio.Fetcher{
			Client: failingClient{client},
		},
	}
	uv.InjectDecoder(decoder)
	return uv
}

func testNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func podFromResources(name, namespace string, res podResource, modify ...func(*corev1.Pod)) *corev1.Pod {
	p := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	for i, cr := range res {
		c := corev1.Container{
			Name: fmt.Sprintf("container-%d", i),
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{},
			},
		}
		if cr.cpu != "" {
			c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cr.cpu)
		}
		if cr.memory != "" {
			c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(cr.memory)
		}
		p.Spec.Containers = append(p.Spec.Containers, c)
	}

	for _, m := range modify {
		m(&p)
	}

	return &p
}

func deploymentFromResources(name, namespace string, replicas int32, res podResource, modify ...func(*appsv1.Deployment)) *appsv1.Deployment {
	deploy := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	deploy.Spec.Replicas = &replicas
	deploy.Spec.Template.Spec.Containers = newTestContainers(res)

	for _, m := range modify {
		m(&deploy)
	}

	return &deploy
}

func statefulsetFromResources(name, namespace string, replicas int32, res podResource, modify ...func(*appsv1.StatefulSet)) *appsv1.StatefulSet {
	sts := appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	sts.Spec.Replicas = &replicas
	sts.Spec.Template.Spec.Containers = newTestContainers(res)

	for _, m := range modify {
		m(&sts)
	}

	return &sts
}

func newTestContainers(res []containerResources) []corev1.Container {
	var containers []corev1.Container
	for _, cr := range res {
		container := corev1.Container{
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{},
			},
		}
		if cr.cpu != "" {
			container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cr.cpu)
		}
		if cr.memory != "" {
			container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(cr.memory)
		}
		containers = append(containers, container)
	}
	return containers
}

type podResource []containerResources
type containerResources struct {
	cpu    string
	memory string
}

type failingClient struct {
	client.WithWatch
}

func (c failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	lo := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(lo)
	}
	if strings.HasPrefix(lo.Namespace, "fail-") {
		return apierrors.NewInternalError(errors.New("ups"))
	}
	return c.WithWatch.List(ctx, list, opts...)
}

func requireParseQuantity(t *testing.T, s string) *resource.Quantity {
	t.Helper()
	q, err := resource.ParseQuantity(s)
	require.NoError(t, err)
	return &q
}
