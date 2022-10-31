package controllers

import (
	"context"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"testing"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

func TestOrganizationRBACReconciler(t *testing.T) {
	recorder := record.NewFakeRecorder(4)
	_, err := prepareOranizationRBACTest(t, testOrganizationRBACfg{
		recorder: recorder,
	}).Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNs.Name,
			Name:      testPod.Name,
		},
	})
	assert.NoError(t, err)
}

type testOrganizationRBACfg struct {
	obj      []client.Object
	recorder record.EventRecorder
}

func prepareOranizationRBACTest(t *testing.T, cfg testOrganizationRBACfg) *OrganizationRBACReconciler {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cfg.obj...).
		Build()

	if cfg.recorder == nil {
		cfg.recorder = &record.FakeRecorder{}
	}

	return &OrganizationRBACReconciler{
		Client:   client,
		Recorder: cfg.recorder,
		Scheme:   scheme,
	}
}
