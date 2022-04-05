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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRatioValidator_Handle(t *testing.T) {
	ctx := context.Background()
	tests := map[string]struct {
		user       string
		namespace  string
		resources  []client.Object
		object     client.Object
		create     bool
		limit      string
		warn       bool
		fail       bool
		statusCode int32
	}{

		"Allow_EmptyNamespace": {
			user:      "appuio#foo",
			namespace: "foo",
			limit:     "4Gi",
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
			limit: "4Gi",
			warn:  false,
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
			limit: "1Gi",
			warn:  true,
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
			limit: "1Gi",
			warn:  false,
		},
		"ListFailure": {
			user:      "bar",
			namespace: "fail-bar",
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
			limit:      "1Gi",
			warn:       false,
			fail:       true,
			statusCode: http.StatusInternalServerError,
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
			limit:  "4Gi",
			warn:   true,
			create: true,
		},
		"Warn_ConsiderNewDeployment": {
			user:      "appuio#foo",
			namespace: "foo",
			resources: []client.Object{
				podFromResources("pod1", "foo", podResource{
					{cpu: "0", memory: "4Gi"},
				}),
			},
			object: deploymentFromResources("unfair", "foo", 2, podResource{
				{cpu: "1", memory: "1Gi"},
			}),
			limit:  "4Gi",
			warn:   true,
			create: true,
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
			limit:  "4Gi",
			warn:   true,
			create: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			v := prepareTest(t, tc.resources...)
			limit := resource.MustParse(tc.limit)
			v.RatioLimit = &limit

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

				ar.AdmissionRequest.Object = runtime.RawExtension{
					Raw: raw,
				}

			}
			if tc.create {
				ar.AdmissionRequest.Operation = admissionv1.Create
			}

			resp := v.Handle(ctx, ar)
			if tc.fail {
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
			} else {
				assert.Empty(t, resp.AdmissionResponse.Warnings)
			}
		})
	}
}

func prepareTest(t *testing.T, initObjs ...client.Object) *RatioValidator {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	decoder, err := admission.NewDecoder(scheme)
	require.NoError(t, err)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		Build()

	uv := &RatioValidator{}
	uv.InjectClient(failingClient{
		WithWatch: client,
	})
	uv.InjectDecoder(decoder)
	return uv
}

func podFromResources(name, namespace string, res podResource) *corev1.Pod {
	p := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
	return &p
}

func deploymentFromResources(name, namespace string, replicas int32, res podResource) *appsv1.Deployment {
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
	return &deploy
}
func statefulsetFromResources(name, namespace string, replicas int32, res podResource) *appsv1.StatefulSet {
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
	return &sts
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
