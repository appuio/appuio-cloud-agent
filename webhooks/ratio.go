package webhooks

import (
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ratio struct {
	cpu    *resource.Quantity
	memory *resource.Quantity
}

func (r *ratio) recordPod(pods ...corev1.Pod) *ratio {
	if r.memory == nil {
		r.memory = &resource.Quantity{}
	}
	if r.cpu == nil {
		r.cpu = &resource.Quantity{}
	}
	for _, pod := range pods {
		for _, c := range pod.Spec.Containers {
			r.memory.Add(*c.Resources.Requests.Memory())
			r.cpu.Add(*c.Resources.Requests.Cpu())
		}
	}
	return r
}

func (r ratio) ratio() *resource.Quantity {
	if r.cpu.IsZero() {
		return resource.NewQuantity(math.MaxInt64, resource.BinarySI)
	}
	return resource.NewQuantity(int64(r.memory.AsApproximateFloat64()/r.cpu.AsApproximateFloat64()), resource.BinarySI)
}

func (r ratio) below(limit resource.Quantity) bool {
	return r.ratio().Cmp(limit) < 0
}

func (r ratio) String() string {
	return r.ratio().String()
}
