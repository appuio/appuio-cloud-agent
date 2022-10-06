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

	"github.com/appuio/appuio-cloud-agent/skipper"
	"github.com/appuio/appuio-cloud-agent/validate"
)

func Test_NamespaceNodeSelectorValidator_Handle(t *testing.T) {
	allowed := &validate.AllowedLabels{}
	require.NoError(t, allowed.Add("appuio.io/node-class", "flex|plus"))

	subject := NamespaceNodeSelectorValidator{
		AllowedNodeSelectors: allowed,
		Skipper:              skipper.StaticSkipper{},
	}
	require.NoError(t, subject.InjectDecoder(decoder(t)))

	testCases := []struct {
		name        string
		annotations map[string]string
		allowed     bool
	}{
		{"no node selector", nil, true},
		{"allowed node selector", map[string]string{OpenshiftNodeSelectorAnnotation: "appuio.io/node-class=flex"}, true},
		{"disallowed node selector", map[string]string{OpenshiftNodeSelectorAnnotation: "appuio.io/node-class=premium"}, false},
		{"invalid node selector", map[string]string{OpenshiftNodeSelectorAnnotation: "??"}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := subject.Handle(context.Background(), admissionRequestForObject(t, newNamespace("test", nil, tc.annotations)))
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.Equal(t, tc.allowed, resp.Allowed)
		})
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

func decoder(t *testing.T) *admission.Decoder {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	decoder, err := admission.NewDecoder(scheme)
	require.NoError(t, err)

	return decoder
}
