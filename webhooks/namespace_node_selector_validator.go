package webhooks

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NamespaceNodeSelectorValidator checks namespaces for allowed node selectors.
type NamespaceNodeSelectorValidator struct {
	decoder *admission.Decoder
}

// Handle handles the admission requests
func (v *NamespaceNodeSelectorValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.validate-namespace-node-selector.appuio.io").
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
