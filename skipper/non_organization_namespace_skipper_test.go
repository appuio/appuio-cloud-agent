package skipper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func Test_NonOrganizationNamespaceSkipper_Skip(t *testing.T) {

	crs := []client.Object{
		&corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "namespace-without-org",
			},
		},
		&corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "org-namespace",
				Labels: map[string]string{
					"appuio.io/organization": "org",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crs...).Build()

	subject := NonOrganizationNamespaceSkipper{
		Client: c,

		OrganizationLabel: "appuio.io/organization",
	}

	testCases := []struct {
		name             string
		admissionRequest admissionv1.AdmissionRequest
		skipped          bool
		errors           bool
	}{
		{
			name: "namespace without organization label",
			admissionRequest: admissionv1.AdmissionRequest{
				Namespace: "namespace-without-org",
			},
			skipped: true,
		}, {
			name: "namespace with organization label",
			admissionRequest: admissionv1.AdmissionRequest{
				Namespace: "org-namespace",
			},
			skipped: false,
		}, {
			name: "non existing namespace",
			admissionRequest: admissionv1.AdmissionRequest{
				Namespace: "non-existing-namespace",
			},
			skipped: false,
			errors:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			skipped, err := subject.Skip(context.Background(), admission.Request{
				AdmissionRequest: tc.admissionRequest,
			})
			if tc.errors {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.skipped, skipped)
		})
	}
}
