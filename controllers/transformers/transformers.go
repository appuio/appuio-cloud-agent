package transformers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Transformer interface {
	Transform(ctx context.Context, u *unstructured.Unstructured, namespace *corev1.Namespace) error
}
