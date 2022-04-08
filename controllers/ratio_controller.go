package controllers

import (
	"context"
	"strconv"

	"github.com/appuio/appuio-cloud-agent/webhooks"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RatioReconciler reconciles a Pod object
type RatioReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme

	RatioLimit        *resource.Quantity
  OrganizationLabel string
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

var eventReason = "TooMuchCPURequest"
var eventMessage = "Memory to CPU ratio of %s/core in this namespace is low"

// Reconcile
func (r *RatioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)

	ns := corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{
		Name: req.Namespace,
	}, &ns)
	if err != nil {
		l.Error(err, "failed to get namespace")
		return ctrl.Result{}, err
	}

	if _, ok := ns.Labels[r.OrganizationLabel]; !ok {
		l.V(1).Info("namespace ignored")
		return ctrl.Result{}, nil
	}
	disabled, ok := ns.Annotations[webhooks.RatioValidatiorDisableAnnotation]
	if ok {
		d, err := strconv.ParseBool(disabled)
		if err == nil && d {
			l.V(1).Info("warnings disabled")
			return ctrl.Result{}, nil
		}
	}

	ratio, err := r.getRatio(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	l = l.WithValues("ratio", ratio.String())

	if ratio.Below(*r.RatioLimit) {
		l.Info("recording warn event: ratio too low")
		pod := corev1.Pod{}
		err := r.Get(ctx, req.NamespacedName, &pod)
		if err != nil {
			l.Error(err, "failed to get pod")
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&ns, "Warning", eventReason, eventMessage, ratio.String())
		r.Recorder.Eventf(&pod, "Warning", eventReason, eventMessage, ratio.String())
	}

	return ctrl.Result{}, nil
}
func (r *RatioReconciler) getRatio(ctx context.Context, ns string) (*webhooks.Ratio, error) {
	ratio := webhooks.NewRatio()
	pods := corev1.PodList{}
	err := r.List(ctx, &pods, client.InNamespace(ns))
	if err != nil {
		return ratio, err
	}
	return ratio.RecordPod(pods.Items...), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RatioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
