package controllers

import (
	"context"
	"testing"

	controlv1 "github.com/appuio/control-api/apis/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	cloudagentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
)

func Test_ZoneUsageProfileSyncReconciler_Reconcile(t *testing.T) {
	upstream := controlv1.UsageProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: controlv1.UsageProfileSpec{
			NamespaceCount: 33,
			Resources: map[string]runtime.RawExtension{
				"cpu": {},
			},
		},
	}

	recorder := record.NewFakeRecorder(4)
	client, scheme, _ := prepareClient(t)
	foreignClient, _, _ := prepareClient(t, &upstream)

	subject := ZoneUsageProfileSyncReconciler{
		Client:        client,
		Scheme:        scheme,
		Recorder:      recorder,
		ForeignClient: foreignClient,
	}

	_, err := subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: upstream.Name}})
	require.NoError(t, err)
	var synced cloudagentv1.ZoneUsageProfile
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: upstream.Name}, &synced))
	require.Equal(t, upstream.Spec, synced.Spec.UpstreamSpec)
	require.Equal(t, "Normal Reconciled Reconciled ZoneUsageProfile: created", <-recorder.Events)

	upstream.Spec.NamespaceCount += 1
	require.NoError(t, foreignClient.Update(context.Background(), &upstream))
	_, err = subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: upstream.Name}})
	require.NoError(t, err)
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: upstream.Name}, &synced))
	require.Equal(t, upstream.Spec, synced.Spec.UpstreamSpec, "downstream should have been updated from upstream")
	require.Equal(t, "Normal Reconciled Reconciled ZoneUsageProfile: updated", <-recorder.Events)
}

func prepareClient(t *testing.T, initObjs ...client.Object) (client.WithWatch, *runtime.Scheme, *admission.Decoder) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, cloudagentv1.AddToScheme(scheme))
	require.NoError(t, controlv1.AddToScheme(scheme))

	decoder := admission.NewDecoder(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		Build()

	return client, scheme, decoder
}
