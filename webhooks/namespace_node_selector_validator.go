package webhooks

import (
	"context"
	"fmt"

	"github.com/appuio/appuio-cloud-agent/skipper"
	"github.com/appuio/appuio-cloud-agent/validate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-namespace-node-selector,name=validate-namespace-node-selector.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups="",resources=namespaces,verbs=create;update,versions=v1,matchPolicy=equivalent

const OpenshiftNodeSelectorAnnotation = "openshift.io/node-selector"

// NamespaceNodeSelectorValidator checks namespaces for allowed node selectors.
type NamespaceNodeSelectorValidator struct {
	decoder *admission.Decoder

	Skipper              skipper.Skipper
	AllowedNodeSelectors *validate.AllowedLabels
}

// Handle handles the admission requests
func (v *NamespaceNodeSelectorValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.validate-namespace-node-selector.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind)

	if req.Kind.Group != "" || req.Kind.Kind != "Namespace" {
		l.V(1).Info("allowed: not a namespace")
		return admission.Allowed("not a namespace")
	}

	skip, err := v.Skipper.Skip(req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(500, err)
	}
	if skip {
		l.V(1).Info("allowed: skipped")
		return admission.Allowed("skipped")
	}

	ns := corev1.Namespace{}
	if err := v.decoder.Decode(req, &ns); err != nil {
		l.Error(err, "failed to decode request")
		return admission.Errored(400, err)
	}

	rawSel := ns.Annotations[OpenshiftNodeSelectorAnnotation]
	if rawSel == "" {
		l.V(1).Info("allowed: no node selector")
		return admission.Allowed("no node selector")
	}

	sel, err := labels.ConvertSelectorToLabelsMap(rawSel)
	if err != nil {
		l.Error(err, "failed to decode "+OpenshiftNodeSelectorAnnotation)
		return admission.Errored(400, err)
	}

	if err := v.AllowedNodeSelectors.Validate(sel); err != nil {
		l.V(1).Info("denied: node selector not allowed", "err", err)
		return admission.Denied(fmt.Sprintf("node selector not allowed: %s", err.Error()))
	}

	l.V(1).Info("allowed: valid node selector")
	return admission.Allowed("valid node selector")
}

// InjectDecoder injects a Admission request decoder
func (v *NamespaceNodeSelectorValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
