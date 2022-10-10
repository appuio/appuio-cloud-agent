package webhooks

import (
	"context"
	"fmt"
	"strings"

	"github.com/appuio/appuio-cloud-agent/skipper"
	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-pod-node-selector,name=mutate-pod-node-selector.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=true,failurePolicy=Fail,groups="",resources=pods,verbs=create;update,versions=v1,matchPolicy=equivalent
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// PodNodeSelectorMutator checks namespaces for allowed node selectors.
type PodNodeSelectorMutator struct {
	decoder *admission.Decoder

	// Client is used to fetch namespace metadata
	Client client.Reader

	// DefaultNodeSelector is the default node selector to apply to pods
	DefaultNodeSelector map[string]string
	// DefaultNamespaceNodeSelectorAnnotation is the annotation to use for the default node selector
	DefaultNamespaceNodeSelectorAnnotation string

	Skipper skipper.Skipper
}

// Handle handles the admission requests
func (v *PodNodeSelectorMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.mutate-pod-node-selector.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name,
			"group", req.Kind.Group, "version", req.Kind.Version, "kind", req.Kind.Kind)

	if req.Kind.Group != "" || req.Kind.Kind != "Pod" {
		l.V(1).Info("wrong kind", "group", req.Kind.Group, "kind", req.Kind.Kind)
		return admission.Errored(400, fmt.Errorf("expected a Pod, got a %s", req.Kind.Kind))
	}

	skip, err := v.Skipper.Skip(ctx, req)
	if err != nil {
		l.Error(err, "error while checking skipper")
		return admission.Errored(500, err)
	}
	if skip {
		l.V(1).Info("allowed: skipped")
		return admission.Allowed("skipped")
	}

	var ns corev1.Namespace
	if err := v.Client.Get(ctx, client.ObjectKey{Name: req.Namespace}, &ns); err != nil {
		l.Error(err, "error while fetching namespace")
		return admission.Errored(500, err)
	}

	var rawPod unstructured.Unstructured
	if err := v.decoder.Decode(req, &rawPod); err != nil {
		l.Error(err, "failed to decode request")
		return admission.Errored(400, err)
	}

	var defaults labels.Set
	rawDefaults := ns.Annotations[v.DefaultNamespaceNodeSelectorAnnotation]
	if v.DefaultNamespaceNodeSelectorAnnotation == "" || rawDefaults == "" {
		if len(v.DefaultNodeSelector) == 0 {
			l.V(1).Info("allowed: no default selector")
			return admission.Allowed("no default selector")
		}
		defaults = labels.Set(v.DefaultNodeSelector)
	} else {
		d, err := labels.ConvertSelectorToLabelsMap(rawDefaults)
		if err != nil {
			l.Error(err, "failed to decode "+v.DefaultNamespaceNodeSelectorAnnotation)
			return admission.Errored(400, err)
		}
		defaults = d
	}

	nodeSel, hasNodeSel, err := unstructured.NestedStringMap(rawPod.Object, "spec", "nodeSelector")
	if err != nil {
		l.Error(err, "failed to check for existing nodeSelector")
		return admission.Errored(500, err)
	}

	patches := make([]jsonpatch.Operation, 0, len(defaults)+1)
	if hasNodeSel {
		for k, v := range defaults {
			if _, exists := nodeSel[k]; !exists {
				patches = append(patches, jsonpatch.NewOperation("add", "/spec/nodeSelector/"+escapeJSONPointer(k), v))
			}
		}
	} else {
		patches = append(patches, jsonpatch.Operation{
			Operation: "add",
			Path:      "/spec/nodeSelector",
			Value:     defaults,
		})
	}

	l.V(1).Info("built patch", "nodeSelector", nodeSel, "defaults", defaults, "patch", patches)
	return admission.Patched("added default node selector", patches...)
}

func escapeJSONPointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// InjectDecoder injects a Admission request decoder
func (v *PodNodeSelectorMutator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
