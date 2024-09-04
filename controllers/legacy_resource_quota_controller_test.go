package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_LegacyResourceQuotaReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	subjectNamespace := newNamespace("test", map[string]string{"organization": "testorg"}, nil)

	c, scheme, recorder := prepareClient(t, subjectNamespace)
	ctx := log.IntoContext(context.Background(), testr.New(t))

	subject := LegacyResourceQuotaReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: recorder,

		OrganizationLabel: "organization",

		ResourceQuotaAnnotationBase: "resourcequota.example.com",
		DefaultResourceQuotas: map[string]corev1.ResourceQuotaSpec{
			"orgq": {
				Hard: corev1.ResourceList{
					corev1.ResourceLimitsCPU:       resource.MustParse("10"),
					corev1.ResourceRequestsMemory:  resource.MustParse("10Gi"),
					"count/services.loadbalancers": resource.MustParse("10"),
					"localblock-storage.storageclass.storage.k8s.io/persistentvolumeclaims": resource.MustParse("10"),
					"cephfs-fspool-cluster.storageclass.storage.k8s.io/requests.storage":    resource.MustParse("10"),
					"openshift.io/imagestreamtags":                                          resource.MustParse("10"),
				},
			},
		},

		LimitRangeName: "limitrange",
		DefaultLimitRange: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceLimitsCPU: resource.MustParse("1"),
					},
				},
			},
		},
	}

	_, err := subject.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: subjectNamespace.Name}})
	require.NoError(t, err)

	var syncedRQ corev1.ResourceQuota
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "orgq", Namespace: "test"}, &syncedRQ))
	require.Equal(t, subject.DefaultResourceQuotas["orgq"], syncedRQ.Spec)

	var syncedLR corev1.LimitRange
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "limitrange", Namespace: "test"}, &syncedLR))
	require.Equal(t, subject.DefaultLimitRange, syncedLR.Spec)

	subjectNamespace.Annotations = map[string]string{
		"resourcequota.example.com/orgq.storageclasses":               `{"cephfs-fspool-cluster.storageclass.storage.k8s.io/requests.storage":"5"}`,
		"resourcequota.example.com/orgq.limits.cpu":                   "5",
		"resourcequota.example.com/orgq.count_services.loadbalancers": "5",
		"resourcequota.example.com/orgq.openshift.io_imagestreamtags": "5",
	}
	require.NoError(t, c.Update(ctx, subjectNamespace))

	_, err = subject.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: subjectNamespace.Name}})
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "orgq", Namespace: "test"}, &syncedRQ))
	assert.Equal(t, "5", ptr.To(syncedRQ.Spec.Hard[corev1.ResourceLimitsCPU]).String())
	assert.Equal(t, "5", ptr.To(syncedRQ.Spec.Hard["count/services.loadbalancers"]).String())
	assert.Equal(t, "5", ptr.To(syncedRQ.Spec.Hard["openshift.io/imagestreamtags"]).String())
	assert.Equal(t, "5", ptr.To(syncedRQ.Spec.Hard["cephfs-fspool-cluster.storageclass.storage.k8s.io/requests.storage"]).String())
	assert.Equal(t, "10", ptr.To(syncedRQ.Spec.Hard["localblock-storage.storageclass.storage.k8s.io/persistentvolumeclaims"]).String())
	assert.Equal(t, "10Gi", ptr.To(syncedRQ.Spec.Hard[corev1.ResourceRequestsMemory]).String())
}
