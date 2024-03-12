package controllers

import (
	"context"
	"testing"

	controlv1 "github.com/appuio/control-api/apis/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_GroupSyncReconciler_Reconcile(t *testing.T) {
	upstreamTeam := controlv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "developers",
			Namespace: "thedoening",
		},
		Spec: controlv1.TeamSpec{
			UserRefs: buildUserRefs("johndoe"),
		},
		Status: controlv1.TeamStatus{
			ResolvedUserRefs: buildUserRefs("johndoe"),
		},
	}
	upstreamOM := controlv1.OrganizationMembers{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OrganizationMembersManifestName,
			Namespace: "thedoening",
		},
		Spec: controlv1.OrganizationMembersSpec{
			UserRefs: buildUserRefs("johndoe"),
		},
		Status: controlv1.OrganizationMembersStatus{
			ResolvedUserRefs: buildUserRefs("johndoe"),
		},
	}

	client, scheme, recorder := prepareClient(t)
	foreignClient, _, _ := prepareClient(t, &upstreamTeam, &upstreamOM)

	subject := GroupSyncReconciler{
		Client:        client,
		Scheme:        scheme,
		Recorder:      recorder,
		ForeignClient: foreignClient,

		ControlAPIFinalizerZoneName: "lupfig",
	}

	t.Run("Team", func(t *testing.T) {
		// Create
		_, err := subject.Reconcile(context.Background(), teamMapper(context.Background(), &upstreamTeam)[0])
		require.NoError(t, err)
		var group userv1.Group
		require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: "thedoening+developers"}, &group), "should have created a group from the team")
		require.Equal(t, userv1.OptionalNames{"johndoe"}, group.Users, "should have set the group users")
		// Finalizer
		require.NoError(t, foreignClient.Get(context.Background(), namespacedName(&upstreamTeam), &upstreamTeam))
		require.Contains(t, upstreamTeam.Finalizers, "agent.appuio.io/group-zone-lupfig", "should have added a finalizer upstream")

		// Update
		upstreamTeam.Spec.UserRefs = buildUserRefs("johndoe", "janedoe")
		upstreamTeam.Status.ResolvedUserRefs = buildUserRefs("johndoe", "janedoe")
		require.NoError(t, foreignClient.Update(context.Background(), &upstreamTeam))
		_, err = subject.Reconcile(context.Background(), teamMapper(context.Background(), &upstreamTeam)[0])
		require.NoError(t, err)
		require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: "thedoening+developers"}, &group))
		require.Equal(t, userv1.OptionalNames{"janedoe", "johndoe"}, group.Users, "should have updated the group from the team")

		// Delete upstream team
		require.NoError(t, foreignClient.Delete(context.Background(), &upstreamTeam))
		require.NoError(t, foreignClient.Get(context.Background(), namespacedName(&upstreamTeam), &upstreamTeam), "should not have deleted the upstream team since it has a finalizer")
		_, err = subject.Reconcile(context.Background(), teamMapper(context.Background(), &upstreamTeam)[0])
		require.NoError(t, err)
		require.True(t, apierrors.IsNotFound(foreignClient.Get(context.Background(), namespacedName(&upstreamTeam), &upstreamTeam)), "should have deleted the upstream team after removing the finalizer")
	})

	t.Run("OrganizationMembers", func(t *testing.T) {
		// Create
		_, err := subject.Reconcile(context.Background(), organizationMembersMapper(context.Background(), &upstreamOM)[0])
		require.NoError(t, err)
		var group userv1.Group
		require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: "thedoening"}, &group), "should have created a group from the organization members")
		require.Equal(t, userv1.OptionalNames{"johndoe"}, group.Users, "should have set the group users")
		// Finalizer
		require.NoError(t, foreignClient.Get(context.Background(), namespacedName(&upstreamOM), &upstreamOM))
		require.Contains(t, upstreamOM.Finalizers, "agent.appuio.io/group-zone-lupfig", "should have added a finalizer upstream")

		// Update
		upstreamOM.Spec.UserRefs = buildUserRefs("johndoe", "janedoe")
		upstreamOM.Status.ResolvedUserRefs = buildUserRefs("johndoe", "janedoe")
		require.NoError(t, foreignClient.Update(context.Background(), &upstreamOM))
		_, err = subject.Reconcile(context.Background(), organizationMembersMapper(context.Background(), &upstreamOM)[0])
		require.NoError(t, err)
		require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: "thedoening"}, &group))
		require.Equal(t, userv1.OptionalNames{"janedoe", "johndoe"}, group.Users, "should have updated the group from the organization members")

		// Delete upstream organization members
		require.NoError(t, foreignClient.Delete(context.Background(), &upstreamOM))
		require.NoError(t, foreignClient.Get(context.Background(), namespacedName(&upstreamOM), &upstreamOM), "should not have deleted the upstream OrganizationMembers since it has a finalizer")
		_, err = subject.Reconcile(context.Background(), organizationMembersMapper(context.Background(), &upstreamOM)[0])
		require.NoError(t, err)
		require.True(t, apierrors.IsNotFound(foreignClient.Get(context.Background(), namespacedName(&upstreamOM), &upstreamOM)), "should have deleted the upstream OrganizationMembers after removing the finalizer")
	})
}

func buildUserRefs(names ...string) []controlv1.UserRef {
	var refs []controlv1.UserRef
	for _, name := range names {
		refs = append(refs, controlv1.UserRef{Name: name})
	}
	return refs
}

func namespacedName(o client.Object) types.NamespacedName {
	return types.NamespacedName{Name: o.GetName(), Namespace: o.GetNamespace()}
}
