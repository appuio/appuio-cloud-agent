package testutils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureGroupVersionKind ensures that the object has a valid GroupVersionKind set.
// It does modify the object and also returns the modified object for convenience.
func EnsureGroupVersionKind(t *testing.T, scheme *runtime.Scheme, obj client.Object) client.Object {
	t.Helper()

	if !obj.GetObjectKind().GroupVersionKind().Empty() {
		return obj
	}

	gvk, err := findGVKForObject(scheme, obj)
	require.NoError(t, err)
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return obj
}

func findGVKForObject(scheme *runtime.Scheme, obj client.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	for _, gvk := range gvks {
		if gvk.Kind == "" {
			continue
		}
		if gvk.Version == "" || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		return gvk, nil
	}
	return schema.GroupVersionKind{}, fmt.Errorf("no valid GVK found for object %T", obj)
}
