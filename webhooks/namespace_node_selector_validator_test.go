package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/validate"
)

func Test_NamespaceNodeSelectorValidator_Handle(t *testing.T) {
	allowed := &validate.AllowedLabels{}
	require.NoError(t, allowed.Add("appuio.io/node-class", "flex|plus"))

	subject := NamespaceNodeSelectorValidator{
		allowedNodeSelectors: allowed,
	}
	require.NoError(t, subject.InjectDecoder(decoder(t)))

	var resp admission.Response

	resp = subject.Handle(context.Background(), admissionRequestForObject(t, newNamespace("test", nil, nil)))
	require.True(t, resp.Allowed)

	resp = subject.Handle(context.Background(), admissionRequestForObject(t, newNamespace("test", nil, map[string]string{
		OpenshiftNodeSelectorAnnotation: "appuio.io/node-class=flex",
	})))
	require.True(t, resp.Allowed)

	resp = subject.Handle(context.Background(), admissionRequestForObject(t, newNamespace("test", nil, map[string]string{
		OpenshiftNodeSelectorAnnotation: "???",
	})))
	require.False(t, resp.Allowed)

	resp = subject.Handle(context.Background(), admissionRequestForObject(t, newNamespace("test", nil, map[string]string{
		OpenshiftNodeSelectorAnnotation: "appuio.io/node-class=xxl",
	})))
	require.False(t, resp.Allowed)
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

func decoder(t *testing.T) *admission.Decoder {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	decoder, err := admission.NewDecoder(scheme)
	require.NoError(t, err)

	return decoder
}
