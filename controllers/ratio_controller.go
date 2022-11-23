package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/appuio/appuio-cloud-agent/limits"
	"github.com/appuio/appuio-cloud-agent/ratio"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
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

	Ratio       ratioFetcher
	RatioLimits limits.Limits
}

type ratioFetcher interface {
	FetchRatios(ctx context.Context, ns string) (map[string]*ratio.Ratio, error)
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

var eventReason = "TooMuchCPURequest"

// Reconcile reacts to pod updates and emits events if the fair use request ratio is violated
func (r *RatioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)

	nsRatios, err := r.Ratio.FetchRatios(ctx, req.Namespace)
	if err != nil {
		if errors.Is(err, ratio.ErrorDisabled) {
			l.V(1).Info("namespace disabled")
			return ctrl.Result{}, nil
		}
		l.Error(err, "failed to get ratio")
		return ctrl.Result{}, err
	}

	for nodeSel, ratio := range nsRatios {
		// TODO() add nodeSel to warning message

		sel, err := labels.ConvertSelectorToLabelsMap(nodeSel)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to convert node selector '%s' to labels map: %w", nodeSel, err)
		}
		limit := r.RatioLimits.GetLimitForNodeSelector(sel)
		if limit == nil {
			l.Info("no limit found for node selector", "nodeSelector", nodeSel)
			continue
		}

		if ratio.Below(*limit) {
			l.Info("recording warn event: ratio too low")

			if err := r.warnPod(ctx, req.Name, req.Namespace, ratio, limit); err != nil {
				l.Error(err, "failed to record event on pod")
			}
			if err := r.warnNamespace(ctx, req.Namespace, ratio, limit); err != nil {
				l.Error(err, "failed to record event on namespace")
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *RatioReconciler) warnPod(ctx context.Context, name, namespace string, nsRatio *ratio.Ratio, limit *resource.Quantity) error {
	pod := corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &pod)
	if err != nil {
		return err
	}
	r.Recorder.Event(&pod, "Warning", eventReason, nsRatio.Warn(limit))
	return nil
}
func (r *RatioReconciler) warnNamespace(ctx context.Context, name string, nsRatio *ratio.Ratio, limit *resource.Quantity) error {
	ns := corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{
		Name: name,
	}, &ns)
	if err != nil {
		return err
	}
	r.Recorder.Event(&ns, "Warning", eventReason, nsRatio.Warn(limit))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RatioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
