package controllers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/controllers/transformers"
)

// ZoneUsageProfileApplyReconciler reconciles a ZoneUsageProfile object.
// It applies the resources defined in the ZoneUsageProfile to all namespaces with the given organization label.
// It dynamically watches the resources defined in the ZoneUsageProfile to keep those resources in sync.
// Controller and Cache need to be set before using this reconciler to setup the dynamic watch.
// Controller is set by the SetupWithManager function.
type ZoneUsageProfileApplyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	Controller controller.Controller
	Cache      cache.Cache

	// watchTracker keeps track of which resources are already being watched.
	watchTracker sync.Map

	OrganizationLabel string
	Transformers      []transformers.Transformer
}

const resourceOwnerLabel = "cloud-agent.appuio.io/usage-profile"

//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudagent.appuio.io,resources=zoneusageprofiles/finalizers,verbs=update

// Reconcile applies a ZoneUsageProfile to all namespaces with the given organization label.
// It returns an error if more than one ZoneUsageProfile try to manage a resource.
func (r *ZoneUsageProfileApplyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("Reconciling ZoneUsageProfile")

	var profile cloudagentv1.ZoneUsageProfile
	if err := r.Client.Get(ctx, req.NamespacedName, &profile); err != nil {
		l.Error(err, "unable to get ZoneUsageProfile")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var orgNsl corev1.NamespaceList
	if err := r.Client.List(ctx, &orgNsl, client.HasLabels{r.OrganizationLabel}); err != nil {
		l.Error(err, "unable to list Namespaces")
		return ctrl.Result{}, err
	}

	var errors []error
	for _, orgNs := range orgNsl.Items {
		l := l.WithValues("namespace", orgNs.Name)
		l.Info("Applying UsageProfile to Namespace")
		for name, resource := range profile.Spec.UpstreamSpec.Resources {
			l := l.WithValues("resourceName", name)
			l.Info("Applying UsageProfile Resource to Namespace")

			if err := r.applyResourceToNamespace(ctx, name, orgNs, resource, profile); err != nil {
				l.Error(err, "unable to create or update resource")
				r.Recorder.Event(&profile, "Warning", "ApplyFailed", fmt.Sprintf("unable to create or update resource %q in %q: %s", name, orgNs.Name, err))
				errors = append(errors, err)
			}
		}
	}

	return ctrl.Result{}, multierr.Combine(errors...)
}

// applyResourceToNamespace applies a resource from a ZoneUsageProfile to a namespace.
// It handles the needed conversions and sets the resourceOwnerLabel.
// It returns an error if the resource is already managed by a different ZoneUsageProfile.
func (r *ZoneUsageProfileApplyReconciler) applyResourceToNamespace(ctx context.Context, name string, orgNs corev1.Namespace, resource runtime.RawExtension, profile cloudagentv1.ZoneUsageProfile) error {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&resource)
	if err != nil {
		return fmt.Errorf("unable to convert RawExtension to Unstructured: %w", err)
	}
	u := &unstructured.Unstructured{}
	// Set enough information for the GET request to work
	u.SetGroupVersionKind((&unstructured.Unstructured{Object: raw}).GetObjectKind().GroupVersionKind())
	u.SetNamespace(orgNs.Name)
	u.SetName(name)
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, u, func() error {
		lbls := u.GetLabels()
		if lbls == nil {
			lbls = make(map[string]string)
		}

		// Set the full object to be applied, this will overwrite the existing object so we need to re-set the metadata.
		u.SetUnstructuredContent(raw)
		u.SetNamespace(orgNs.Name)
		u.SetName(name)
		err := controllerutil.SetControllerReference(&profile, u, r.Scheme)
		if err != nil {
			return fmt.Errorf("unable to set controller reference: %w", err)
		}

		for _, t := range r.Transformers {
			if err := t.Transform(ctx, u, &orgNs); err != nil {
				log.FromContext(ctx).Error(err, "unable to fully transform object")
			}
		}

		p, exists := lbls[resourceOwnerLabel]
		if exists && p != profile.Name {
			return fmt.Errorf("conflict: resource %q/%q in %q already has a different UsageProfile applied: %s", u.GetObjectKind().GroupVersionKind().String(), name, orgNs.Name, p)
		}
		lbls[resourceOwnerLabel] = profile.Name
		u.SetLabels(lbls)
		return nil
	})

	if err := r.ensureWatch(ctx, u.GetObjectKind().GroupVersionKind()); err != nil {
		return fmt.Errorf("unable to watch, object might not be reconciled: %w", err)
	}

	return err
}

// ensureWatch ensures that the given GroupVersionKind is watched exactly once by the controller.
func (r *ZoneUsageProfileApplyReconciler) ensureWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	// TODO(bastjan): we'll need envtest to test dynamic watches
	if r.Cache == nil || r.Controller == nil {
		log.FromContext(ctx).Error(errors.New("no cache or controller available"), "skipping dynamic watch")
		return nil
	}

	if gvk.Empty() {
		return fmt.Errorf("unable to watch empty GroupVersionKind")
	}

	var err error
	once, _ := r.watchTracker.LoadOrStore(gvk, new(sync.Once))
	once.(*sync.Once).Do(func() {
		toWatch := &unstructured.Unstructured{}
		toWatch.SetGroupVersionKind(gvk)

		err = r.Controller.Watch(source.Kind(r.Cache, toWatch), handler.EnqueueRequestForOwner(r.Scheme, r.Client.RESTMapper(), &cloudagentv1.ZoneUsageProfile{}))
	})
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneUsageProfileApplyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	orgPredicate, err := labelExistsPredicate(r.OrganizationLabel)
	if err != nil {
		return fmt.Errorf("unable to create LabelSelectorPredicate: %w", err)
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&cloudagentv1.ZoneUsageProfile{}).
		// Watch all namespaces and enqueue requests for all profiles on any change.
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(mapToAllUsageProfiles(mgr.GetClient())),
			builder.WithPredicates(orgPredicate)).
		Build(r)
	if err != nil {
		return fmt.Errorf("unable to create controller: %w", err)
	}
	r.Controller = c
	return nil
}

// labelExistsPredicate returns a predicate that matches objects with the given label.
func labelExistsPredicate(label string) (predicate.Predicate, error) {
	return predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      label,
			Operator: metav1.LabelSelectorOpExists,
		}}})
}

// mapToAllUsageProfiles returns a MapFunc that enqueues reconcile requests for all ZoneUsageProfiles on every event.
func mapToAllUsageProfiles(cl client.Client) handler.MapFunc {
	return func(ctx context.Context, _ client.Object) []reconcile.Request {
		var profiles cloudagentv1.ZoneUsageProfileList
		if err := cl.List(ctx, &profiles); err != nil {
			log.FromContext(ctx).Error(err, "unable to list ZoneUsageProfiles")
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(profiles.Items))
		for _, profile := range profiles.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKey{Name: profile.Name}})
		}
		return reqs
	}
}
