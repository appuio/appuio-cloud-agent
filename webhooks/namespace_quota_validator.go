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
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=user.openshift.io,resources=users,verbs=get;list;watch

// NamespaceQuotaValidator checks if a user is allowed to create a namespace.
// The user or the namespace must have a label with the organization name.
// The organization name is used to count the number of namespaces for the organization.
type NamespaceQuotaValidator struct {
	Decoder admission.Decoder

	// Client is used to fetch namespace counts
	Client client.Reader

	Skipper skipper.Skipper

	// SkipValidateQuota allows skipping the quota validation.
	// If the validation is skipped only the organization label is checked.
	SkipValidateQuota bool

	OrganizationLabel                 string
	UserDefaultOrganizationAnnotation string

	// SelectedProfile is the name of the ZoneUsageProfile to use for the quota
	SelectedProfile string

	// QuotaOverrideNamespace is the namespace in which the quota overrides are stored
	QuotaOverrideNamespace string
}

// Handle handles the admission requests
func (v *NamespaceQuotaValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).
		WithName("webhook.validate-namespace-quota.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind))

	return logAdmissionResponse(ctx, v.handle(ctx, req))
}

func (v *NamespaceQuotaValidator) handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx)

	skip, err := v.Skipper.Skip(ctx, req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if skip {
		return admission.Allowed("skipped")
	}

	var rawObject unstructured.Unstructured
	if err := v.Decoder.Decode(req, &rawObject); err != nil {
		l.Error(err, "failed to decode request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	// try to get the organization name from a namespace object.
	// Note: ProjectRequest labels are ignored by the API server so only the user default organization can be used.
	var organizationName string
	if rawObject.GetKind() == "Namespace" {
		on, _, err := unstructured.NestedString(rawObject.Object, "metadata", "labels", v.OrganizationLabel)
		if err != nil {
			l.Error(err, "error while fetching organization label")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		organizationName = on
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
		l.Info("got default organization from user", "user", req.UserInfo.Username, "organization", organizationName)
	}

	if v.SkipValidateQuota {
		return admission.Allowed("skipped quota validation")
	}

	if v.SelectedProfile == "" {
		return admission.Denied("No ZoneUsageProfile selected")
	}

	var profile cloudagentv1.ZoneUsageProfile
	if err := v.Client.Get(ctx, types.NamespacedName{Name: v.SelectedProfile}, &profile); err != nil {
		l.Error(err, "error while fetching zone usage profile")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	nsCountLimit := profile.Spec.UpstreamSpec.NamespaceCount

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
		return admission.Denied(fmt.Sprintf(
			"You cannot create more than %d namespaces for organization %q. Please contact support to have your quota raised.",
			nsCountLimit, organizationName))
	}

	return admission.Allowed("allowed")
}

// logAdmissionResponse logs the admission response to the logger derived from the given context and returns it unchanged.
func logAdmissionResponse(ctx context.Context, res admission.Response) admission.Response {
	l := log.FromContext(ctx)

	rmsg := "<not given>"
	if res.Result != nil {
		rmsg = res.Result.Message
	}
	msg := "denied"
	if res.Allowed {
		msg = "allowed"
	}

	l.Info(msg, "admission_message", rmsg)

	return res
}
