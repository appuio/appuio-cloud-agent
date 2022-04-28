package ratio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

type deployResource struct {
	containers []containerResources
	replicas   int32
}
type podResource []containerResources
type containerResources struct {
	cpu    string
	memory string
}
