package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// RatioValidator checks for every action in a namespace whether the Memory to CPU ratio limit is exceeded and will return a warning if it is.
type RatioValidator struct {
	client  client.Client
	decoder *admission.Decoder

	RatioLimit *resource.Quantity
}

// Handle handles the admission requests
func (v *RatioValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if strings.HasPrefix(req.UserInfo.Username, "system:") {
		// Is service account or kube system user: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects
		return admission.Allowed("system user")
	}

	l := log.FromContext(ctx).WithName("webhook.validate-request-ratio.appuio.io")
	l.V(3).WithValues("kind", req.Kind.Kind, "namespace", req.Namespace).Info("handling request")

	r, err := v.getRatio(ctx, req.Namespace)
	if err != nil {
		return errored(http.StatusInternalServerError, err)
	}

	// If we are creating an object with resource requests, we add them to the current ratio
	// We cannot easily do this when updating resources.
	if req.Operation == admissionv1.Create {
		switch req.Kind.Kind {
		case "Pod":
			pod := corev1.Pod{}
			if err := v.decoder.Decode(req, &pod); err != nil {
				return errored(http.StatusBadRequest, err)
			}
			r = r.RecordPod(pod)
		case "Deployment":
			deploy := appsv1.Deployment{}
			if err := v.decoder.Decode(req, &deploy); err != nil {
				return errored(http.StatusBadRequest, err)
			}
			r = r.RecordDeployment(deploy)
		case "StatefulSet":
			sts := appsv1.StatefulSet{}
			if err := v.decoder.Decode(req, &sts); err != nil {
				return errored(http.StatusBadRequest, err)
			}
			r = r.RecordStatefulSet(sts)
		}
	}

	if r.Below(*v.RatioLimit) {
		return admission.Response{
			AdmissionResponse: admissionv1.AdmissionResponse{
				Allowed:  true,
				Warnings: []string{fmt.Sprintf("Current memory to CPU ratio of %s/core is below the fair use ratio of %s/core", r, v.RatioLimit)},
			}}
	}
	return admission.Allowed("ok")
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
