package webhooks

import (
	"context"
	"fmt"
	"net/http"

	userv1 "github.com/openshift/api/user/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

// +kubebuilder:webhook:path=/validate-namespace-quota,name=validate-namespace-quota.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups="",resources=namespaces,verbs=create,versions=v1,matchPolicy=equivalent
// +kubebuilder:webhook:path=/validate-namespace-quota,name=validate-namespace-quota-projectrequests.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups=project.openshift.io,resources=projectrequests,verbs=create,versions=v1,matchPolicy=equivalent
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// NamespaceQuotaValidator checks namespaces for allowed node selectors.
type NamespaceQuotaValidator struct {
	decoder *admission.Decoder

	// Client is used to fetch namespace counts
	Client client.Reader

	Skipper skipper.Skipper

	OrganizationLabel                 string
	UserDefaultOrganizationAnnotation string

	DefaultNamespaceCountLimit int
}

// Handle handles the admission requests
func (v *NamespaceQuotaValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.validate-namespace-quota.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind)

	skip, err := v.Skipper.Skip(ctx, req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if skip {
		l.V(1).Info("allowed: skipped")
		return admission.Allowed("skipped")
	}

	// try to get the organization name from the namespace/projectrequest
	var rawObject unstructured.Unstructured
	if err := v.decoder.Decode(req, &rawObject); err != nil {
		l.Error(err, "failed to decode request")
		return admission.Errored(http.StatusBadRequest, err)
	}
	organizationName, _, err := unstructured.NestedString(rawObject.Object, "metadata", "labels", v.OrganizationLabel)
	if err != nil {
		l.Error(err, "error while fetching organization label")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// get the organization name from the user if it is not set on the namespace/projectrequest
	if organizationName == "" {
		var user userv1.User
		if err := v.Client.Get(ctx, client.ObjectKey{Name: req.UserInfo.Username}, &user); err != nil {
			l.Error(err, "error while fetching user")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		don := user.Annotations[v.UserDefaultOrganizationAnnotation]
		if don == "" {
			return admission.Denied("There is no organization label and the user has no default organization set.")
		}
		organizationName = don
	}

	// count namespaces for organization
	var nsList corev1.NamespaceList
	if err := v.Client.List(ctx, &nsList, client.MatchingLabels{
		v.OrganizationLabel: organizationName,
	}); err != nil {
		l.Error(err, "error while listing namespaces")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if len(nsList.Items) >= v.DefaultNamespaceCountLimit {
		l.V(1).Info("denied: namespace count limit reached", "limit", v.DefaultNamespaceCountLimit, "count", len(nsList.Items))
		return admission.Denied(fmt.Sprintf(
			"You cannot create more than %d namespaces for organization %q. Please contact support to have your quota raised.",
			v.DefaultNamespaceCountLimit, organizationName))
	}

	return admission.Allowed("allowed")
}

// InjectDecoder injects a Admission request decoder
func (v *NamespaceQuotaValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
