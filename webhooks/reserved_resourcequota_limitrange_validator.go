package webhooks

import (
	"context"
	"net/http"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

// +kubebuilder:webhook:path=/validate-reserved-resourcequota-limitrange,name=reserved-resourcequota-limitrange-validator.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups="",resources=resourcequotas;limitranges,verbs=create;update;delete,versions=v1,matchPolicy=equivalent

// ReservedResourceQuotaLimitRangeValidator denies changes to reserved resourcequota and limitrange objects.
type ReservedResourceQuotaLimitRangeValidator struct {
	Decoder admission.Decoder

	Skipper skipper.Skipper

	ReservedResourceQuotaNames []string
	ReservedLimitRangeNames    []string
}

// Handle handles the admission requests
func (v *ReservedResourceQuotaLimitRangeValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).
		WithName("webhook.reserved-resourcequota-limitrange-validator.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind))

	return logAdmissionResponse(ctx, v.handle(ctx, req))
}

func (v *ReservedResourceQuotaLimitRangeValidator) handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx)

	skip, err := v.Skipper.Skip(ctx, req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if skip {
		return admission.Allowed("skipped")
	}

	if req.Kind.Kind == "ResourceQuota" {
		if slices.Contains(v.ReservedResourceQuotaNames, req.Name) {
			return admission.Denied("reserved ResourceQuota object")
		}
	}

	if req.Kind.Kind == "LimitRange" {
		if slices.Contains(v.ReservedLimitRangeNames, req.Name) {
			return admission.Denied("reserved LimitRange object")
		}
	}

	return admission.Allowed("allowed")
}
