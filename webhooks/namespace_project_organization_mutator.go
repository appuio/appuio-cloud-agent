package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/appuio/appuio-cloud-agent/skipper"
	userv1 "github.com/openshift/api/user/v1"
	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/mutate-namespace-project-organization,name=namespace.namespace-project-organization-mutator.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=true,failurePolicy=Fail,groups="",resources=namespaces,verbs=create;update,versions=v1,matchPolicy=equivalent
//+kubebuilder:webhook:path=/mutate-namespace-project-organization,name=project.namespace-project-organization-mutator.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=true,failurePolicy=Fail,groups=project.openshift.io,resources=projects,verbs=create;update,versions=v1,matchPolicy=equivalent

// +kubebuilder:rbac:groups=user.openshift.io,resources=users;groups,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// NamespaceProjectOrganizationMutator adds the OrganizationLabel to namespace and project create requests.
type NamespaceProjectOrganizationMutator struct {
	Decoder admission.Decoder

	Client client.Reader

	Skipper skipper.Skipper

	// OrganizationLabel is the label used to mark namespaces to belong to an organization
	OrganizationLabel string

	// UserDefaultOrganizationAnnotation is the annotation the default organization setting for a user is stored in.
	UserDefaultOrganizationAnnotation string
}

const OpenShiftProjectRequesterAnnotation = "openshift.io/requester"

// Handle handles the admission requests
//
// If the requestor is a service account:
//   - Project requests are denied.
//   - Namespace requests are checked against the organization of the service account's namespace.
//   - If the organization is not set in the request, the organization of the service account's namespace is added.
//   - If the service account's namespace has no organization set, the request is denied.
//
// If the requestor is an OpenShift user:
// - If there is no OrganizationLabel set on the object, the default organization of the user is used; if there is no default organization set for the user, the request is denied.
// - Namespace requests use the username of the requests user info.
// - Project requests use the annotation `openshift.io/requester` on the project object. If the annotation is not set, the request is allowed.
// - If the user is not a member of the organization, the request is denied; this is done by checking for an OpenShift group with the same name as the organization.
func (m *NamespaceProjectOrganizationMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).
		WithName("webhook.namespace-project-organization-mutator.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind))

	return logAdmissionResponse(ctx, m.handle(ctx, req))
}

func (m *NamespaceProjectOrganizationMutator) handle(ctx context.Context, req admission.Request) admission.Response {
	skip, err := m.Skipper.Skip(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error while checking skipper: %w", err))
	}
	if skip {
		return admission.Allowed("skipped")
	}

	var rawObject unstructured.Unstructured
	if err := m.Decoder.Decode(req, &rawObject); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode object from request: %w", err))
	}

	if req.Kind.Kind == "Project" {
		return m.handleProject(ctx, rawObject)
	}

	return m.handleNamespace(ctx, req, rawObject)
}

func (m *NamespaceProjectOrganizationMutator) handleProject(ctx context.Context, rawObject unstructured.Unstructured) admission.Response {
	userName := rawObject.GetAnnotations()[OpenShiftProjectRequesterAnnotation]
	if userName == "" {
		// https://github.com/appuio/component-appuio-cloud/blob/196f76ede357a73b88f9314bf1d1bccc097cb6b7/component/namespace-policies.jsonnet#L54
		return admission.Allowed("Skipped: no requester annotation found")
	}
	if strings.HasPrefix(userName, "system:serviceaccount:") {
		return admission.Denied("Service accounts are not allowed to create projects")
	}

	return m.handleUserRequested(ctx, userName, rawObject)
}

func (m *NamespaceProjectOrganizationMutator) handleNamespace(ctx context.Context, req admission.Request, rawObject unstructured.Unstructured) admission.Response {
	if strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:") {
		return m.handleServiceAccountNamespace(ctx, req, rawObject)
	}

	return m.handleUserRequested(ctx, req.UserInfo.Username, rawObject)
}

func (m *NamespaceProjectOrganizationMutator) handleUserRequested(ctx context.Context, userName string, rawObject unstructured.Unstructured) admission.Response {
	var user userv1.User
	if err := m.Client.Get(ctx, client.ObjectKey{Name: userName}, &user); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to get user %s: %w", userName, err))
	}

	org := rawObject.GetLabels()[m.OrganizationLabel]
	defaultOrgAdded := false
	if org == "" {
		org = user.Annotations[m.UserDefaultOrganizationAnnotation]
		defaultOrgAdded = true
	}
	if org == "" {
		return admission.Denied("No organization label found and no default organization set")
	}

	var group userv1.Group
	if err := m.Client.Get(ctx, types.NamespacedName{Name: org}, &group); client.IgnoreNotFound(err) != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to get group: %w", err))
	}

	if !slices.Contains(group.Users, userName) {
		return admission.Denied("Requester is not a member of the organization")
	}

	if defaultOrgAdded {
		return admission.Patched("added default organization", m.orgLabelPatch(org))
	}

	return admission.Allowed("Requester is member of organization")
}

func (m *NamespaceProjectOrganizationMutator) handleServiceAccountNamespace(ctx context.Context, req admission.Request, rawObject unstructured.Unstructured) admission.Response {
	p := strings.Split(req.UserInfo.Username, ":")
	if len(p) != 4 {
		return admission.Errored(http.StatusUnprocessableEntity, fmt.Errorf("invalid service account name: %s, expected 4 segments", req.UserInfo.Username))
	}
	nsName := p[2]
	var ns corev1.Namespace
	if err := m.Client.Get(ctx, client.ObjectKey{Name: nsName}, &ns); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to get namespace %s for service account %s: %w", nsName, req.UserInfo.Username, err))
	}
	nsOrg := ns.Labels[m.OrganizationLabel]
	if nsOrg == "" {
		return admission.Denied("No organization label found for the service accounts namespace")
	}

	requestedOrg := rawObject.GetLabels()[m.OrganizationLabel]
	if requestedOrg != "" && requestedOrg != nsOrg {
		return admission.Denied("Service accounts are not allowed to use organizations other than the one of their namespace.")
	}

	if requestedOrg == "" {
		return admission.Patched("added organization label", m.orgLabelPatch(nsOrg))
	}

	return admission.Allowed("service account may use the organization of its namespace")
}

// orgLabelPatch returns a JSON patch operation to add the `OrganizationLabel` with value `org` to an object.
func (m *NamespaceProjectOrganizationMutator) orgLabelPatch(org string) jsonpatch.Operation {
	return jsonpatch.Operation{
		Operation: "add",
		Path:      "/" + strings.Join([]string{"metadata", "labels", escapeJSONPointerSegment(m.OrganizationLabel)}, "/"),
		Value:     org,
	}
}
