package webhooks

import (
	"context"

	admissionv1 "k8s.io/api/admission/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// UserValidator holds context for the validating admission webhook for users.appuio.io
type RatioValidator struct {
	client  client.Client
	decoder *admission.Decoder
}

// Handle handles the users.appuio.io admission requests
func (v *RatioValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.FromContext(ctx).WithName("webhook.validate-request-ratio.appuio.io")
	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed:  true,
			Warnings: []string{"Not Implemented"},
		},
	}
}

// InjectDecoder injects a Admission request decoder into the UserValidator
func (v *RatioValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// InjectClient injects a Kubernetes client into the UserValidator
func (v *RatioValidator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}
