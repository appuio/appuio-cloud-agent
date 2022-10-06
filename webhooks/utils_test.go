package webhooks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func admissionRequestForObject(t *testing.T, object client.Object) admission.Request {
	t.Helper()

	gvk := object.GetObjectKind().GroupVersionKind()

	raw, err := json.Marshal(object)
	require.NoError(t, err)

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
		},
	}
}
