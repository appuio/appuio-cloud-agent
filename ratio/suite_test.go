package ratio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func assertResourceEqual(t *testing.T, res *resource.Quantity, s string) bool {
	return assert.Truef(t, res.Equal(resource.MustParse(s)), "%s should be equal to %s", res, s)
}

func newTestContainers(res []containerResources) []corev1.Container {
	var containers []corev1.Container
	for _, cr := range res {
		container := corev1.Container{
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{},
			},
		}
		if cr.cpu != "" {
			container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cr.cpu)
		}
		if cr.memory != "" {
			container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(cr.memory)
		}
		containers = append(containers, container)
	}
	return containers
}

func testNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

type failingClient struct {
	client.WithWatch
}

func (c failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	lo := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(lo)
	}
	if strings.HasPrefix(lo.Namespace, "fail-") {
		return apierrors.NewInternalError(errors.New("ups"))
	}
	return c.WithWatch.List(ctx, list, opts...)
}

func podFromResources(name, namespace string, res podResource) *corev1.Pod {
	p := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	for i, cr := range res {
		c := corev1.Container{
			Name: fmt.Sprintf("container-%d", i),
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{},
			},
		}
		if cr.cpu != "" {
			c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cr.cpu)
		}
		if cr.memory != "" {
			c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(cr.memory)
		}
		p.Spec.Containers = append(p.Spec.Containers, c)
	}
	return &p
}

type deployResource struct {
	containers []containerResources
	replicas   int32
}
type podResource []containerResources
type containerResources struct {
	cpu    string
	memory string
}
