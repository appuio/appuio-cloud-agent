package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// LegacyResourceQuotaReconciler reconciles namespaces and synchronizes their resource quotas
type LegacyResourceQuotaReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	OrganizationLabel string

	ResourceQuotaAnnotationBase string
	DefaultResourceQuotas       map[string]corev1.ResourceQuotaSpec

	LimitRangeName    string
	DefaultLimitRange corev1.LimitRangeSpec
}

func (r *LegacyResourceQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling Namespace")

	var ns corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if ns.DeletionTimestamp != nil {
		l.Info("Namespace is being deleted, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if _, ok := ns.Labels[r.OrganizationLabel]; !ok {
		l.Info("Namespace does not have organization label, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	var errs []error
	for name, s := range r.DefaultResourceQuotas {
		spec := *s.DeepCopy()

		var storageQuotas corev1.ResourceList
		if sqa := ns.Annotations[fmt.Sprintf("%s/%s.storageclasses", r.ResourceQuotaAnnotationBase, name)]; sqa != "" {
			err := json.Unmarshal([]byte(ns.Annotations[fmt.Sprintf("%s/%s.storageclasses", r.ResourceQuotaAnnotationBase, name)]), &storageQuotas)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to unmarshal storage classes: %w", err))
				storageQuotas = make(corev1.ResourceList)
			}
		} else {
			storageQuotas = make(corev1.ResourceList)
		}

		rq := &corev1.ResourceQuota{
			ObjectMeta: ctrl.ObjectMeta{
				Name:      name,
				Namespace: ns.Name,
			},
		}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, rq, func() error {
			for k := range spec.Hard {
				an := fmt.Sprintf("%s/%s.%s", r.ResourceQuotaAnnotationBase, name, strings.ReplaceAll(string(k), "/", "_"))
				if strings.Contains(string(k), "storageclass.storage.k8s.io") {
					if _, ok := storageQuotas[k]; ok {
						spec.Hard[k] = storageQuotas[k]
					}
				} else if a := ns.Annotations[an]; a != "" {
					po, err := resource.ParseQuantity(a)
					if err != nil {
						errs = append(errs, fmt.Errorf("failed to parse quantity %s=%s: %w", an, a, err))
						continue
					}
					spec.Hard[k] = po
				}
			}

			rq.Spec = spec
			return controllerutil.SetControllerReference(&ns, rq, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to reconcile ResourceQuota %s: %w", name, err))
		}
		if op != controllerutil.OperationResultNone {
			l.Info("Reconciled ResourceQuota", "name", name, "operation", op)
		}
	}

	lr := &corev1.LimitRange{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      r.LimitRangeName,
			Namespace: ns.Name,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, lr, func() error {
		lr.Spec = *r.DefaultLimitRange.DeepCopy()
		return controllerutil.SetControllerReference(&ns, lr, r.Scheme)
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to reconcile LimitRange %s: %w", r.LimitRangeName, err))
	}
	if op != controllerutil.OperationResultNone {
		l.Info("Reconciled LimitRange", "name", r.LimitRangeName, "operation", op)
	}

	if err := multierr.Combine(errs...); err != nil {
		r.Recorder.Eventf(&ns, corev1.EventTypeWarning, "ReconcileError", "Failed to reconcile ResourceQuotas and LimitRanges: %s", err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to reconcile ResourceQuotas and LimitRanges: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LegacyResourceQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	orgPredicate, err := labelExistsPredicate(r.OrganizationLabel)
	if err != nil {
		return fmt.Errorf("failed to create organization label predicate: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("legacyresourcequota").
		For(&corev1.Namespace{}, builder.WithPredicates(orgPredicate)).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Complete(r)
}
