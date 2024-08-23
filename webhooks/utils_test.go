package webhooks

import (
	"encoding/json"
	"testing"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/testutils"
)

func admissionRequestForObject(t *testing.T, object client.Object, scheme *runtime.Scheme) admission.Request {
	t.Helper()

	return admissionRequestForObjectWithOldObject(t, object, nil, scheme)
}

func admissionRequestForObjectWithOldObject(t *testing.T, object, oldObject client.Object, scheme *runtime.Scheme) admission.Request {
	t.Helper()

	testutils.EnsureGroupVersionKind(t, scheme, object)
	gvk := object.GetObjectKind().GroupVersionKind()

	raw, err := json.Marshal(object)
	require.NoError(t, err)

	var oldRaw []byte
	if oldObject != nil {
		testutils.EnsureGroupVersionKind(t, scheme, oldObject)
		r, err := json.Marshal(oldObject)
		require.NoError(t, err)
		oldRaw = r
	}

	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "e515f52d-7181-494d-a3d3-f0738856bd97",
			Kind: metav1.GroupVersionKind{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
			},
			Name:      object.GetName(),
			Namespace: object.GetNamespace(),
			Operation: admissionv1.Update,
			UserInfo: authenticationv1.UserInfo{
				Username: "user",
				Groups: []string{
					"oidc:user",
				},
			},
			Object: runtime.RawExtension{
				Raw: raw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
}

func newNamespace(name string, labels, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func newProjectRequest(name string, labels, annotations map[string]string) *projectv1.ProjectRequest {
	return &projectv1.ProjectRequest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Project",
			APIVersion: "project.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func newService(name string, labels, annotations map[string]string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func prepareClient(t *testing.T, initObjs ...client.Object) (client.WithWatch, *runtime.Scheme, admission.Decoder) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, userv1.AddToScheme(scheme))
	require.NoError(t, projectv1.AddToScheme(scheme))
	require.NoError(t, cloudagentv1.AddToScheme(scheme))

	decoder := admission.NewDecoder(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		Build()

	return client, scheme, decoder
}

func newPod(namespace, name string, nodeSelector map[string]string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeSelector: nodeSelector,
		},
	}
}
