package controllers

import (
	"context"
	"testing"

	controlv1 "github.com/appuio/control-api/apis/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

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

	client, scheme, recorder := prepareClient(t)
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
