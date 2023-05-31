package transformers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewResourceQuotaTransformer returns a new Transformer that transforms ResourceQuota objects from the annotation of the given namespace.
// The annotation must start with the given labelBase. labelBase is normalized to end with a "/".
func NewResourceQuotaTransformer(labelBase string) Transformer {
	return &resourceQuotaTransformer{
		LabelBase: strings.TrimSuffix(labelBase, "/") + "/",
	}
}

type resourceQuotaTransformer struct {
	LabelBase string
}

var resourceQuotaSpecHardPath = []string{"spec", "hard"}

var resourceQuotaV1gvk = corev1.SchemeGroupVersion.WithKind("ResourceQuota")

func (l *resourceQuotaTransformer) Transform(ctx context.Context, u *unstructured.Unstructured, contextNs *corev1.Namespace) error {
	// only transform v1.ResourceQuota
	if u.GetObjectKind().GroupVersionKind() != resourceQuotaV1gvk {
		return nil
	}

	parsed, parseErr := l.overridesFromObj(contextNs)
	ovs, ok := parsed[u.GetName()]
	// no overrides for this resource
	if !ok {
		return parseErr
	}

	errors := []error{parseErr}
	for _, ov := range ovs {
		errors = append(errors,
			unstructured.SetNestedField(u.UnstructuredContent(), ov.v, append(resourceQuotaSpecHardPath, ov.k)...))
	}

	return multierr.Combine(errors...)
}

// overridesFromObj returns a map of resource names to a slice of key-value pairs.
// An error is returned if parsing was only partially successful.
func (l *resourceQuotaTransformer) overridesFromObj(obj client.Object) (map[string][]kv, error) {
	overrides := make(map[string][]kv)
	base := l.LabelBase

	var errors []error

	for k, v := range obj.GetAnnotations() {
		if !strings.HasPrefix(k, l.LabelBase) {
			continue
		}

		objName, key, ok := strings.Cut(strings.TrimPrefix(k, base), ".")
		if !ok {
			errors = append(errors, fmt.Errorf("invalid annotation %q", k))
			continue
		}

		if key == "storageclasses" {
			var conf map[string]string
			if err := json.Unmarshal([]byte(v), &conf); err != nil {
				errors = append(errors, fmt.Errorf("failed to unmarshal storageclasses: %w", err))
				continue
			}
			for k, v := range conf {
				if strings.Contains(k, "storageclass.storage.k8s.io") {
					overrides[objName] = append(overrides[objName], kv{k: k, v: v})
				}
			}
			continue
		}

		overrides[objName] = append(overrides[objName], kv{k: strings.ReplaceAll(key, "_", "/"), v: v})
	}

	return overrides, multierr.Combine(errors...)
}

type kv struct {
	k string
	v string
}
