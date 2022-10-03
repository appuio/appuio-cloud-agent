package webhooks

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-workload-node-selector,name=validate-workload-node-selector.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups=*,resources=daemonsets;deployments;jobs;statefulsets;cronjobs,verbs=create;update,versions=*,matchPolicy=equivalent

// WorkloadNodeSelectorValidator checks workloads for allowed node selectors.
type WorkloadNodeSelectorValidator struct {
	decoder *admission.Decoder
}

// Handle handles the admission requests
func (v *WorkloadNodeSelectorValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.validate-workload-node-selector.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name, "kind", req.Kind.Kind)

	if strings.HasPrefix(req.UserInfo.Username, "system:") {
		// Is service account or kube system user: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects
		l.V(1).Info("allowed: system user")
		return admission.Allowed("system user")
	}

	l.V(1).Info("allowed: not yet implemented")
	return admission.Allowed("not yet implemented")
}

// InjectDecoder injects a Admission request decoder
func (v *WorkloadNodeSelectorValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
