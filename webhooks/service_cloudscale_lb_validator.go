package webhooks

import (
	"context"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

// +kubebuilder:webhook:path=/validate-service-cloudscale-lb,name=validate-service-cloudscale-lb.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups="",resources=services,verbs=create;update,versions=v1,matchPolicy=equivalent

const (
	CloudscaleLoadbalancerUUIDAnnotation = "k8s.cloudscale.ch/loadbalancer-uuid"
)

// ServiceCloudscaleLBValidator denies changes to the k8s.cloudscale.ch/loadbalancer-uuid annotation.
type ServiceCloudscaleLBValidator struct {
	Decoder admission.Decoder

	Skipper skipper.Skipper
}

// Handle handles the admission requests
func (v *ServiceCloudscaleLBValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).
		WithName("webhook.validate-namespace-quota.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind))

	return logAdmissionResponse(ctx, v.handle(ctx, req))
}

func (v *ServiceCloudscaleLBValidator) handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx)

	skip, err := v.Skipper.Skip(ctx, req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if skip {
		return admission.Allowed("skipped")
	}

	var newService corev1.Service
	if err := v.Decoder.Decode(req, &newService); err != nil {
		l.Error(err, "failed to decode request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	var oldService corev1.Service
	if req.OldObject.Raw != nil {
		if err := v.Decoder.DecodeRaw(req.OldObject, &oldService); err != nil {
			l.Error(err, "failed to decode old object")
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	oldAnnotiation := oldService.GetAnnotations()[CloudscaleLoadbalancerUUIDAnnotation]
	newAnnotiation := newService.GetAnnotations()[CloudscaleLoadbalancerUUIDAnnotation]

	if oldAnnotiation != newAnnotiation {
		l.Info("Loadbalancer UUID changed", "old", oldAnnotiation, "new", newAnnotiation)
		return admission.Denied(fmt.Sprintf("%s annotation cannot be changed", CloudscaleLoadbalancerUUIDAnnotation))
	}

	return admission.Allowed("allowed")
}
