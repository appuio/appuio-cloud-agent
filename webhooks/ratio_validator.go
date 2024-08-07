package webhooks

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"gopkg.in/inf.v0"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/appuio/appuio-cloud-agent/limits"
	"github.com/appuio/appuio-cloud-agent/ratio"
)

// +kubebuilder:webhook:path=/validate-request-ratio,name=validate-request-ratio.appuio.io,admissionReviewVersions=v1,sideEffects=none,mutating=false,failurePolicy=ignore,groups=*,resources=*,verbs=create;update,versions=*,matchPolicy=equivalent

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// RatioValidator checks for every action in a namespace whether the Memory to CPU ratio limit is exceeded and will return a warning if it is.
type RatioValidator struct {
	Decoder admission.Decoder
	Client  client.Client

	Ratio              ratioFetcher
	RatioLimits        limits.Limits
	RatioWarnThreshold *inf.Dec

	// DefaultNodeSelector is the default node selector to apply to pods
	DefaultNodeSelector map[string]string
	// DefaultNamespaceNodeSelectorAnnotation is the annotation to use for the default node selector
	DefaultNamespaceNodeSelectorAnnotation string
}

type ratioFetcher interface {
	FetchRatios(ctx context.Context, ns string) (map[string]*ratio.Ratio, error)
}

// Handle handles the admission requests
func (v *RatioValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := log.FromContext(ctx).
		WithName("webhook.validate-request-ratio.appuio.io").
		WithValues("id", req.UID, "user", req.UserInfo.Username).
		WithValues("namespace", req.Namespace, "name", req.Name, "kind", req.Kind.Kind)

	if strings.HasPrefix(req.UserInfo.Username, "system:") {
		// Is service account or kube system user: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects
		l.V(1).Info("allowed: system user")
		return admission.Allowed("system user")
	}

	ratios, err := v.Ratio.FetchRatios(ctx, req.Namespace)
	if err != nil {
		if errors.Is(err, ratio.ErrorDisabled) {
			l.V(1).Info("allowed: namespace disabled")
			return admission.Allowed("validation disabled")
		}
		if apierrors.IsNotFound(err) {
			l.Error(err, "namespace not found")
			return errored(http.StatusNotFound, err)
		}
		l.Error(err, "failed to get ratio")
		return errored(http.StatusInternalServerError, err)
	}

	nodeSel, err := v.getNodeSelector(req)
	if err != nil {
		l.Error(err, "failed to get node selector")
		return errored(http.StatusBadRequest, err)
	}
	if len(nodeSel) == 0 {
		sel, err := v.getDefaultNodeSelectorFromNamespace(ctx, req.Namespace)
		if err != nil {
			l.Error(err, "failed to get default node selector from namespace")
		}
		nodeSel = sel
	}
	if len(nodeSel) == 0 {
		nodeSel = v.DefaultNodeSelector
	}

	l = l.WithValues("current_ratios", ratios, "node_selector", nodeSel)
	// If we are creating an object with resource requests, we add them to the current ratio
	// We cannot easily do this when updating resources.
	if req.Operation == admissionv1.Create {
		key := fuzzyMatchRatioKey(labels.Set(nodeSel).String(), ratios)
		r := ratios[key]
		if r == nil {
			r = ratio.NewRatio()
		}
		r, err = v.recordObject(ctx, r, req)
		if err != nil {
			l.Error(err, "failed to record object")
			return errored(http.StatusBadRequest, err)
		}
		ratios[key] = r

		l = l.WithValues("ratio", r)
	}

	warnings := make([]string, 0, len(ratios))
	for nodeSel, r := range ratios {
		sel, err := labels.ConvertSelectorToLabelsMap(nodeSel)
		if err != nil {
			return errored(http.StatusInternalServerError, err)
		}
		limit := v.RatioLimits.GetLimitForNodeSelector(sel)
		if limit == nil {
			l.Info("no limit found for node selector", "nodeSelector", nodeSel)
			continue
		}

		if r.Below(*limit, v.RatioWarnThreshold) {
			l.Info("ratio too low", "node_selector", nodeSel, "ratio", r)
			warnings = append(warnings, r.Warn(limit, nodeSel))
		}
	}

	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed:  true,
			Warnings: warnings,
		},
	}
}

func (v *RatioValidator) recordObject(ctx context.Context, r *ratio.Ratio, req admission.Request) (*ratio.Ratio, error) {
	switch req.Kind.Kind {
	case "Pod":
		pod := corev1.Pod{}
		if err := v.Decoder.Decode(req, &pod); err != nil {
			return r, err
		}
		r = r.RecordPod(pod)
	case "Deployment":
		deploy := appsv1.Deployment{}
		if err := v.Decoder.Decode(req, &deploy); err != nil {
			return r, err
		}
		r = r.RecordDeployment(deploy)
	case "StatefulSet":
		sts := appsv1.StatefulSet{}
		if err := v.Decoder.Decode(req, &sts); err != nil {
			return r, err
		}
		r = r.RecordStatefulSet(sts)
	}
	return r, nil
}

func (v *RatioValidator) getNodeSelector(req admission.Request) (map[string]string, error) {
	switch req.Kind.Kind {
	case "Pod":
		pod := corev1.Pod{}
		if err := v.Decoder.Decode(req, &pod); err != nil {
			return nil, err
		}
		return pod.Spec.NodeSelector, nil
	case "Deployment":
		deploy := appsv1.Deployment{}
		if err := v.Decoder.Decode(req, &deploy); err != nil {
			return nil, err
		}
		return deploy.Spec.Template.Spec.NodeSelector, nil
	case "StatefulSet":
		sts := appsv1.StatefulSet{}
		if err := v.Decoder.Decode(req, &sts); err != nil {
			return nil, err
		}
		return sts.Spec.Template.Spec.NodeSelector, nil
	}
	return nil, nil
}

func (v *RatioValidator) getDefaultNodeSelectorFromNamespace(ctx context.Context, namespace string) (map[string]string, error) {
	ns := corev1.Namespace{}
	err := v.Client.Get(ctx, client.ObjectKey{Name: namespace}, &ns)
	if err != nil {
		return nil, err
	}
	return labels.ConvertSelectorToLabelsMap(ns.Annotations[v.DefaultNamespaceNodeSelectorAnnotation])
}

func errored(code int32, err error) admission.Response {
	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Code:    code,
				Message: err.Error(),
			},
		},
	}
}

// fuzzyMatchRatioKey returns the key in ratios that matches the node selector.
// If there is no exact match, it returns the key that matches a subset of the labels.
// This is done if the node selector comes from the default node selector which does
// not contain all labels that might be added by the scheduler.
func fuzzyMatchRatioKey(s string, ratios map[string]*ratio.Ratio) string {
	if _, ok := ratios[s]; ok {
		return s
	}

	sel, _ := labels.Parse(s)
	for k := range ratios {
		ks, _ := labels.ConvertSelectorToLabelsMap(k)
		if sel.Matches(ks) {
			return k
		}
	}

	return s
}
