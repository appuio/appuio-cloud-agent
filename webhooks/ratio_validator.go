package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// RatioValidator checks for every action in a namespace whether the Memory to CPU ratio limit is exceeded and will return a warning if it is.
type RatioValidator struct {
	client  client.Client
	decoder *admission.Decoder

	RatioLimit *resource.Quantity
}

// RatioValidatiorDisableAnnotation is the key for an annotion on a namespace to disable request ratio warnings
var RatioValidatiorDisableAnnotation = "validate-request-ratio.appuio.io/disable"

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

	disabled, err := v.isNamespaceDisabled(ctx, req.Namespace)
	if err != nil {
		l.Error(err, "failed to get namespace")
		if apierrors.IsNotFound(err) {
			return errored(http.StatusNotFound, err)
		}
		return errored(http.StatusInternalServerError, err)
	}
	if disabled {
		l.V(1).Info("allowed: warning disabled")
		return admission.Allowed("system user")
	}

	r, err := v.getRatio(ctx, req.Namespace)
	if err != nil {
		l.Error(err, "failed to get ratio")
		return errored(http.StatusInternalServerError, err)
	}

	l = l.WithValues("current_ratio", r.String())
	// If we are creating an object with resource requests, we add them to the current ratio
	// We cannot easily do this when updating resources.
	if req.Operation == admissionv1.Create {
		r, err = v.recodObject(ctx, r, req)
		if err != nil {
			l.Error(err, "failed to record object")
			return errored(http.StatusBadRequest, err)
		}
	}
	l = l.WithValues("ratio", r.String())

	if r.Below(*v.RatioLimit) {
		l.Info("warned: ratio too low")
		return admission.Response{
			AdmissionResponse: admissionv1.AdmissionResponse{
				Allowed: true,
				// WARNING(glrf) Warnings MUST NOT contain newlines. K8s will simply drop your warning if you add newlines.
				Warnings: []string{
					fmt.Sprintf("Current memory to CPU ratio of %s/core in this namespace is below the fair use ratio of %s/core. This might lead to additional costs.", r, v.RatioLimit),
				},
			}}
	}
	l.V(1).Info("allowed: ratio ok")
	return admission.Allowed("ok")
}

func (v *RatioValidator) recodObject(ctx context.Context, r *Ratio, req admission.Request) (*Ratio, error) {
	switch req.Kind.Kind {
	case "Pod":
		pod := corev1.Pod{}
		if err := v.decoder.Decode(req, &pod); err != nil {
			return r, err
		}
		r = r.RecordPod(pod)
	case "Deployment":
		deploy := appsv1.Deployment{}
		if err := v.decoder.Decode(req, &deploy); err != nil {
			return r, err
		}
		r = r.RecordDeployment(deploy)
	case "StatefulSet":
		sts := appsv1.StatefulSet{}
		if err := v.decoder.Decode(req, &sts); err != nil {
			return r, err
		}
		r = r.RecordStatefulSet(sts)
	}
	return r, nil
}

func (v *RatioValidator) isNamespaceDisabled(ctx context.Context, nsName string) (bool, error) {
	ns := corev1.Namespace{}
	err := v.client.Get(ctx, client.ObjectKey{
		Name: nsName,
	}, &ns)
	if err != nil {
		return false, err
	}

	disabled, ok := ns.Annotations[RatioValidatiorDisableAnnotation]
	if !ok {
		return false, err
	}
	return strconv.ParseBool(disabled)
}

func (v *RatioValidator) getRatio(ctx context.Context, ns string) (*Ratio, error) {
	r := NewRatio()
	pods := corev1.PodList{}
	err := v.client.List(ctx, &pods, client.InNamespace(ns))
	if err != nil {
		return r, err
	}
	return r.RecordPod(pods.Items...), nil
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

// InjectDecoder injects a Admission request decoder
func (v *RatioValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// InjectClient injects a Kubernetes client
func (v *RatioValidator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}
