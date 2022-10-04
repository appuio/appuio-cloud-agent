package webhooks

import (
	"context"
	"fmt"

	"github.com/appuio/appuio-cloud-agent/skipper"
	"github.com/appuio/appuio-cloud-agent/validate"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-workload-node-selector,name=validate-workload-node-selector.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=Fail,groups=*,resources=pods;daemonsets;deployments;jobs;statefulsets;cronjobs,verbs=create;update,versions=v1,matchPolicy=equivalent

// WorkloadNodeSelectorValidator checks workloads for allowed node selectors.
type WorkloadNodeSelectorValidator struct {
	decoder *admission.Decoder

	Skipper              skipper.Skipper
	AllowedNodeSelectors *validate.AllowedLabels
}

// Handle handles the admission requests
func (v *WorkloadNodeSelectorValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.validate-workload-node-selector.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name, "kind", req.Kind.Kind)

	skip, err := v.Skipper.Skip(req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(500, err)
	}
	if skip {
		l.V(1).Info("allowed: skipped")
		return admission.Allowed("skipped")
	}

	var nodeSelector map[string]string
	var decodeErr error
	// Decode the workload
	switch req.Kind.Kind {
	case "Pod":
		var workload corev1.Pod
		decodeErr = v.decoder.Decode(req, &workload)
		nodeSelector = workload.Spec.NodeSelector
	case "CronJob":
		var workload batchv1.CronJob
		decodeErr = v.decoder.Decode(req, &workload)
		nodeSelector = workload.Spec.JobTemplate.Spec.Template.Spec.NodeSelector
	case "Job":
		var workload batchv1.Job
		decodeErr = v.decoder.Decode(req, &workload)
		nodeSelector = workload.Spec.Template.Spec.NodeSelector
	case "DaemonSet":
		var workload appsv1.DaemonSet
		decodeErr = v.decoder.Decode(req, &workload)
		nodeSelector = workload.Spec.Template.Spec.NodeSelector
	case "Deployment":
		var workload appsv1.Deployment
		decodeErr = v.decoder.Decode(req, &workload)
		nodeSelector = workload.Spec.Template.Spec.NodeSelector
	case "StatefulSet":
		var workload appsv1.StatefulSet
		decodeErr = v.decoder.Decode(req, &workload)
		nodeSelector = workload.Spec.Template.Spec.NodeSelector
	default:
		decodeErr = fmt.Errorf("unknown workload kind %q", req.Kind.Kind)
	}

	if decodeErr != nil {
		l.Error(decodeErr, "error while decoding workload", "kind", req.Kind.Kind)
		return admission.Errored(500, decodeErr)
	}

	if nodeSelector == nil {
		l.V(1).Info("allowed: no node selector")
		return admission.Allowed("no node selector")
	}

	if err := v.AllowedNodeSelectors.Validate(nodeSelector); err != nil {
		l.V(1).Info("denied: node selector not allowed", "err", err)
		return admission.Denied(fmt.Sprintf("node selector not allowed: %s", err.Error()))
	}

	l.V(1).Info("allowed: valid node selector")
	return admission.Allowed("valid node selector")
}

// InjectDecoder injects a Admission request decoder
func (v *WorkloadNodeSelectorValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
