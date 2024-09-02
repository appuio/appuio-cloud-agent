package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

// +kubebuilder:webhook:path=/mutate-pod-run-once-active-deadline,name=pod-run-once-active-deadline-mutator.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=true,failurePolicy=Fail,groups="",resources=pods,verbs=create,versions=v1,matchPolicy=equivalent,reinvocationPolicy=IfNeeded
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// PodRunOnceActiveDeadlineSecondsMutator adds .spec.activeDeadlineSeconds to pods with the restartPolicy set to "OnFailure" or "Never".
type PodRunOnceActiveDeadlineSecondsMutator struct {
	Decoder admission.Decoder

	// Client is used to fetch namespace metadata for the override annotation
	Client client.Reader

	// DefaultNamespaceNodeSelectorAnnotation is the annotation to use for the default node selector
	OverrideAnnotation string

	// DefaultActiveDeadlineSeconds is the default activeDeadlineSeconds to apply to pods
	DefaultActiveDeadlineSeconds int

	Skipper skipper.Skipper
}

// Handle handles the admission requests
func (m *PodRunOnceActiveDeadlineSecondsMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).
		WithName("webhook.pod-run-once-active-deadline-mutator.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("operation", req.Operation).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind))

	return logAdmissionResponse(ctx, m.handle(ctx, req))
}

func (m *PodRunOnceActiveDeadlineSecondsMutator) handle(ctx context.Context, req admission.Request) admission.Response {
	skip, err := m.Skipper.Skip(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error while checking skipper: %w", err))
	}
	if skip {
		return admission.Allowed("skipped")
	}

	var pod corev1.Pod
	if err := m.Decoder.Decode(req, &pod); err != nil {
		return admission.Errored(http.StatusUnprocessableEntity, err)
	}

	if pod.Spec.RestartPolicy != corev1.RestartPolicyOnFailure && pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		return admission.Allowed(fmt.Sprintf("pod restart policy is %q, no activeDeadlineSeconds needed", pod.Spec.RestartPolicy))
	}

	if pod.Spec.ActiveDeadlineSeconds != nil {
		return admission.Allowed("pod already has an activeDeadlineSeconds value")
	}

	var ns corev1.Namespace
	if err := m.Client.Get(ctx, client.ObjectKey{Name: req.Namespace}, &ns); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to fetch namespace for override annotation: %w", err))
	}

	ads := m.DefaultActiveDeadlineSeconds
	msg := fmt.Sprintf("added default activeDeadlineSeconds %d", ads)
	if oa := ns.Annotations[m.OverrideAnnotation]; oa != "" {
		parsed, err := strconv.Atoi(oa)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to parse override annotation %q for namespace %q: %w", oa, req.Namespace, err))
		}
		ads = parsed
		msg = fmt.Sprintf("added activeDeadlineSeconds %d from override annotation %q", ads, m.OverrideAnnotation)
	}

	return admission.Patched(msg, jsonpatch.Operation{
		Operation: "add",
		Path:      "/spec/restartPolicy",
		Value:     ads,
	})
}
