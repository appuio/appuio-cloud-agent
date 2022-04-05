package webhooks

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	inf "gopkg.in/inf.v0"
)

// Ratio records resource requests and can calculate the current memory to CPU request ratio
type Ratio struct {
	cpu    *inf.Dec
	memory *inf.Dec
}

// NewRatio returns an initialized Ratio
func NewRatio() *Ratio {
	return &Ratio{
		cpu:    &inf.Dec{},
		memory: &inf.Dec{},
	}
}

func (r *Ratio) recordReplicatedPodSpec(replicas int32, spec corev1.PodSpec) *Ratio {
	cpu := inf.NewDec(0, 0)
	mem := inf.NewDec(0, 0)
	for _, c := range spec.Containers {
		mem.Add(mem, c.Resources.Requests.Memory().AsDec())
		cpu.Add(cpu, c.Resources.Requests.Cpu().AsDec())
	}
	rep := inf.NewDec(int64(replicas), 0)

	r.cpu.Add(r.cpu, cpu.Mul(cpu, rep))
	r.memory.Add(r.memory, mem.Mul(mem, rep))
	return r
}

// RecordPod collects all requests in the given Pod(s) and adds it to the ratio
func (r *Ratio) RecordPod(pods ...corev1.Pod) *Ratio {
	for _, pod := range pods {
		r.recordReplicatedPodSpec(1, pod.Spec)
	}
	return r
}

// RecordDeployment collects all requests in the given deployment(s) and adds it to the ratio
func (r *Ratio) RecordDeployment(deps ...appsv1.Deployment) *Ratio {
	for _, dep := range deps {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		r.recordReplicatedPodSpec(replicas, dep.Spec.Template.Spec)
	}
	return r
}

// RecordStatefulSet collects all requests in the given StatefulSet(s) and adds it to the ratio
func (r *Ratio) RecordStatefulSet(stss ...appsv1.StatefulSet) *Ratio {
	for _, sts := range stss {
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		r.recordReplicatedPodSpec(replicas, sts.Spec.Template.Spec)
	}
	return r
}

// Ratio returns the memory to CPU ratio of the recorded objects.
// Returns nil if there are no CPU requests.
func (r Ratio) Ratio() *resource.Quantity {
	if r.cpu.Cmp(inf.NewDec(0, 0)) <= 0 {
		return nil
	}
	rDec := inf.NewDec(0, 0).QuoRound(r.memory, r.cpu, 0, inf.RoundHalfEven)
	return resource.NewDecimalQuantity(*rDec, resource.BinarySI)
}

// Below returns if the memory to CPU ratio of the recorded objects is below the given limit.
// Always returns false if no CPU is requested.
func (r Ratio) Below(limit resource.Quantity) bool {
	return r.Ratio() != nil && r.Ratio().Cmp(limit) < 0
}

// String implements Stringer to print ratio
func (r Ratio) String() string {
	return r.Ratio().String()
}
