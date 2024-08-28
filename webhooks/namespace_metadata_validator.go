package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/minio/pkg/wildcard"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

// +kubebuilder:webhook:path=/validate-namespace-metadata,name=validate-namespace-metadata.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups="",resources=namespaces,verbs=create;update,versions=v1,matchPolicy=equivalent
// +kubebuilder:webhook:path=/validate-namespace-metadata,name=validate-namespace-metadata-projectrequests.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups=project.openshift.io,resources=projectrequests,verbs=create;update,versions=v1,matchPolicy=equivalent

// NamespaceMetadataValidator validates the metadata of a namespace.
type NamespaceMetadataValidator struct {
	Decoder admission.Decoder

	Skipper skipper.Skipper

	// ReservedNamespace is a list of namespaces that are reserved and do not count towards the quota.
	// Supports '*' and '?' wildcards.
	ReservedNamespaces []string
	// AllowedAnnotations is a list of annotations that are allowed on the namespace.
	// Supports '*' and '?' wildcards.
	AllowedAnnotations []string
	// AllowedLabels is a list of labels that are allowed on the namespace.
	// Supports '*' and '?' wildcards.
	AllowedLabels []string
}

// Handle handles the admission requests
func (v *NamespaceMetadataValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).
		WithName("webhook.validate-namespace-metadata.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind))

	return logAdmissionResponse(ctx, v.handle(ctx, req))
}

func (v *NamespaceMetadataValidator) handle(ctx context.Context, req admission.Request) admission.Response {
	skip, err := v.Skipper.Skip(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if skip {
		return admission.Allowed("skipped")
	}

	for _, ns := range v.ReservedNamespaces {
		if wildcard.Match(ns, req.Name) {
			return admission.Denied("Changing or creating reserved namespaces is not allowed.")
		}
	}

	var oldObj unstructured.Unstructured
	if len(req.OldObject.Raw) > 0 {
		if err := v.Decoder.DecodeRaw(req.OldObject, &oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode old object: %w", err))
		}
	}

	var newObj unstructured.Unstructured
	if err := v.Decoder.Decode(req, &newObj); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode object from request: %w", err))
	}

	if err := validateChangedMap(oldObj.GetAnnotations(), newObj.GetAnnotations(), v.AllowedAnnotations, "annotation"); err != nil {
		return admission.Denied(formatDeniedMessage(err, "annotations", v.AllowedAnnotations, newObj.GetAnnotations(), oldObj.GetAnnotations()))
	}
	if err := validateChangedMap(oldObj.GetLabels(), newObj.GetLabels(), v.AllowedLabels, "label"); err != nil {
		return admission.Denied(formatDeniedMessage(err, "labels", v.AllowedLabels, newObj.GetLabels(), oldObj.GetLabels()))
	}

	return admission.Allowed("allowed")
}

func formatDeniedMessage(err error, errMapRef string, allowed []string, newMap, oldMap map[string]string) string {
	msg := `The request was denied:
	%v
The following %s can be modified:
	%s
%s given:
	%s
%s before modification:
	%s
`

	return fmt.Sprintf(msg, err, errMapRef, allowed, errMapRef, newMap, errMapRef, oldMap)
}

func validateChangedMap(old, new map[string]string, allowedKeys []string, errObjectRef string) error {
	changed := changedKeys(old, new)
	errs := make([]error, 0, len(changed))
	for _, k := range changed {
		allowed := slices.ContainsFunc(allowedKeys, func(a string) bool { return wildcard.Match(a, k) })
		if !allowed {
			errs = append(errs, fmt.Errorf("%s %q is not allowed to be changed", errObjectRef, k))
		}
	}

	return multierr.Combine(errs...)
}

func changedKeys(a, b map[string]string) []string {
	changed := sets.New[string]()

	for k, v := range a {
		if bV, ok := b[k]; !ok || v != bV {
			changed.Insert(k)
		}
	}
	for k, v := range b {
		if aV, ok := a[k]; !ok || v != aV {
			changed.Insert(k)
		}
	}

	return sets.List(changed)
}
