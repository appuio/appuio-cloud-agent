package transformers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_ResourceQuotaTransformer_Transform(t *testing.T) {
	subject := NewResourceQuotaTransformer("quota.test.io")

	quota := &corev1.ResourceQuota{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ResourceQuota",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-quota",
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				"cephfs-fspool-cluster.storageclass.storage.k8s.io/requests.storage":    resource.MustParse("1"),
				"localblock-storage.storageclass.storage.k8s.io/persistentvolumeclaims": resource.MustParse("1"),
				"count/configmaps":             resource.MustParse("1"),
				"count/jobs.batch":             resource.MustParse("1"),
				"openshift.io/imagestreamtags": resource.MustParse("1"),
				"pods":                         resource.MustParse("1"),
				"requests.cpu":                 resource.MustParse("1"),
				"requests.ephemeral-storage":   resource.MustParse("1"),
			},
		},
	}

	t.Run("no overrides", func(t *testing.T) {
		toTransform := deepCopyToUnstructured(t, quota)
		require.NoError(t, subject.Transform(context.Background(), toTransform, &corev1.Namespace{}))
		require.Equal(t, deepCopyToUnstructured(t, quota), toTransform)
	})

	t.Run("with overrides", func(t *testing.T) {
		toTransform := deepCopyToUnstructured(t, quota)
		require.NoError(t, subject.Transform(context.Background(), toTransform, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"other_annotation": "value",

					"quota.test.io/test-quota.count_configmaps":             "2",
					"quota.test.io/test-quota.count_jobs.batch":             "2",
					"quota.test.io/test-quota.openshift.io_imagestreamtags": "2",
					"quota.test.io/test-quota.pods":                         "2",
					"quota.test.io/test-quota.requests.cpu":                 "2",
					"quota.test.io/test-quota.requests.ephemeral-storage":   "2",

					"quota.test.io/test-quota.storageclasses": `{ "cephfs-fspool-cluster.storageclass.storage.k8s.io/requests.storage": "2", "localblock-storage.storageclass.storage.k8s.io/persistentvolumeclaims": "2" }`,
				},
			},
		}))
		overridden, found, err := unstructured.NestedStringMap(toTransform.Object, "spec", "hard")
		require.NoError(t, err)
		require.True(t, found)
		for key := range overridden {
			assert.Equalf(t, "2", overridden[key], "key %s was not overridden", key)
		}
	})

	t.Run("with overrides and invalid values", func(t *testing.T) {
		toTransform := deepCopyToUnstructured(t, quota)
		require.Error(t, subject.Transform(context.Background(), toTransform, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"quota.test.io/test-quota.pods":           "2",
					"quota.test.io/test-quota":                "2",
					"quota.test.io/test-quota.storageclasses": `garbel`,
				},
			},
		}))
		overriddenPods, _, err := unstructured.NestedString(toTransform.Object, "spec", "hard", "pods")
		require.NoError(t, err)
		assert.Equal(t, "2", overriddenPods, "expected partial override")
		notOverriddenStorage, _, err := unstructured.NestedString(toTransform.Object, "spec", "hard", "cephfs-fspool-cluster.storageclass.storage.k8s.io/requests.storage")
		require.NoError(t, err)
		assert.Equal(t, "1", notOverriddenStorage)
	})

	t.Run("foreign object", func(t *testing.T) {
		toTransform := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
		}
		require.NoError(t, subject.Transform(context.Background(), toTransform, &corev1.Namespace{}))
		require.Equal(t, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
		}, toTransform)
	})
}

func deepCopyToUnstructured(t *testing.T, obj client.Object) *unstructured.Unstructured {
	t.Helper()

	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj.DeepCopyObject())
	require.NoError(t, err)

	return &unstructured.Unstructured{Object: raw}
}
