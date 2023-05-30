package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	userv1 "github.com/openshift/api/user/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
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

	// SelectedProfile is the name of the ZoneUsageProfile to use for the quota
	SelectedProfile string

	// QuotaOverrideNamespace is the namespace in which the quota overrides are stored
	QuotaOverrideNamespace string
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

	nsCountLimit := v.DefaultNamespaceCountLimit
	if v.SelectedProfile != "" {
		var profile cloudagentv1.ZoneUsageProfile
		if err := v.Client.Get(ctx, types.NamespacedName{Name: v.SelectedProfile}, &profile); err != nil {
			l.Error(err, "error while fetching zone usage profile")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		nsCountLimit = profile.Spec.UpstreamSpec.NamespaceCount
		l.Info("using zone usage profile for namespace count limit", "name", profile.Name, "limit", nsCountLimit)
	}

	var overrideCM corev1.ConfigMap
	if err := v.Client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("override-%s", organizationName), Namespace: v.QuotaOverrideNamespace}, &overrideCM); err == nil {
		if overrideCM.Data["namespaceQuota"] != "" {
			nsCountLimit, err = strconv.Atoi(overrideCM.Data["namespaceQuota"])
			if err != nil {
				l.Error(err, "error while parsing namespace quota")
				return admission.Errored(http.StatusInternalServerError, err)
			}
		}
	} else if !apierrors.IsNotFound(err) {
		l.Error(err, "error while fetching override configmap")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// count namespaces for organization
	var nsList corev1.NamespaceList
	if err := v.Client.List(ctx, &nsList, client.MatchingLabels{
		v.OrganizationLabel: organizationName,
	}); err != nil {
		l.Error(err, "error while listing namespaces")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if len(nsList.Items) >= nsCountLimit {
		l.V(1).Info("denied: namespace count limit reached", "limit", nsCountLimit, "count", len(nsList.Items))
		return admission.Denied(fmt.Sprintf(
			"You cannot create more than %d namespaces for organization %q. Please contact support to have your quota raised.",
			nsCountLimit, organizationName))
	}

	return admission.Allowed("allowed")
}

// InjectDecoder injects a Admission request decoder
func (v *NamespaceQuotaValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
