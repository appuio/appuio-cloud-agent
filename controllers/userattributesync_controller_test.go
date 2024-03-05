package controllers

import (
	"context"
	"testing"

	controlv1 "github.com/appuio/control-api/apis/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func Test_UserAttributeSyncReconciler_Reconcile(t *testing.T) {
	upstream := controlv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "johndoe",
		},
		Spec: controlv1.UserSpec{
			Preferences: controlv1.UserPreferences{
				DefaultOrganizationRef: "thedoening",
			},
		},
	}
	onlyUpstream := controlv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "johnupstream",
		},
		Spec: controlv1.UserSpec{
			Preferences: controlv1.UserPreferences{
				DefaultOrganizationRef: "onlyupstream",
			},
		},
	}
	local := userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "johndoe",
		},
	}
	onlyLocal := userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "onlylocal",
		},
	}

	client, scheme, recorder := prepareClient(t, &local, &onlyLocal)
	foreignClient, _, _ := prepareClient(t, &upstream, &onlyUpstream)

	subject := UserAttributeSyncReconciler{
		Client:        client,
		Scheme:        scheme,
		Recorder:      recorder,
		ForeignClient: foreignClient,
	}

	t.Run("normal", func(t *testing.T) {
		_, err := subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: upstream.Name}})
		require.NoError(t, err)
		var synced userv1.User
		require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: upstream.Name}, &synced))
		require.Equal(t, "thedoening", synced.Annotations[DefaultOrganizationAnnotation])
		require.Equal(t, "Normal Reconciled Reconciled User", <-recorder.Events)

		require.Len(t, recorder.Events, 0)
		_, err = subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: upstream.Name}})
		require.NoError(t, err)
		require.Len(t, recorder.Events, 0)
	})

	t.Run("only local", func(t *testing.T) {
		_, err := subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: onlyLocal.Name}})
		require.NoError(t, err)
	})

	t.Run("only upstream", func(t *testing.T) {
		_, err := subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: onlyUpstream.Name}})
		require.NoError(t, err)
	})
}
