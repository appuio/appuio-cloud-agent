package controllers

import (
	"errors"
	"testing"

	controlv1 "github.com/appuio/control-api/apis/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
)

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

func prepareClient(t *testing.T, initObjs ...client.Object) (client.WithWatch, *runtime.Scheme, *record.FakeRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, cloudagentv1.AddToScheme(scheme))
	require.NoError(t, controlv1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		Build()

	return client, scheme, record.NewFakeRecorder(5)
}

// ensureGVK ensures that the object has a valid GVK set.
// It does modify the object and also returns the modified object for convenience.
func ensureGVK(t *testing.T, scheme *runtime.Scheme, obj client.Object) client.Object {
	t.Helper()

	if !obj.GetObjectKind().GroupVersionKind().Empty() {
		return obj
	}

	gvk, err := findGVKForObject(scheme, obj)
	require.NoError(t, err)
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return obj
}

func findGVKForObject(scheme *runtime.Scheme, obj client.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	for _, gvk := range gvks {
		if gvk.Kind == "" {
			continue
		}
		if gvk.Version == "" || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		return gvk, nil
	}
	return schema.GroupVersionKind{}, errors.New("no valid GVK found")
}
