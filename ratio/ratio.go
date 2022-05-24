package ratio

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	inf "gopkg.in/inf.v0"
)

// Ratio records resource requests and can calculate the current memory to CPU request ratio
type Ratio struct {
	CPU    *inf.Dec
	Memory *inf.Dec
}

// NewRatio returns an initialized Ratio
func NewRatio() *Ratio {
	return &Ratio{
		CPU:    &inf.Dec{},
		Memory: &inf.Dec{},
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

	r.CPU.Add(r.CPU, cpu.Mul(cpu, rep))
	r.Memory.Add(r.Memory, mem.Mul(mem, rep))
	return r
}

// RecordPod collects all requests in the given Pod(s), and adds it to the ratio
// The function only considers pods in phase `Running`.
func (r *Ratio) RecordPod(pods ...corev1.Pod) *Ratio {
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning {
			r.recordReplicatedPodSpec(1, pod.Spec)
		}
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
// Ratio rounds up to the nearest MiB
func (r Ratio) Ratio() *resource.Quantity {
	if r.CPU.Cmp(inf.NewDec(0, 0)) <= 0 {
		return nil
	}
	rDec := inf.NewDec(0, 0).QuoRound(r.Memory, r.CPU, 0, inf.RoundHalfEven)
	// Neither infdec nor resource.Quantity provide rounding to powers of
	// two. So we round up to the next MiB by dividing the exact result by
	// 1024*1024, rounding up, and then multiply by 1024*1024 again.
	mib := inf.NewDec(1024*1024, 0)
	rDec.QuoRound(rDec, mib, 0, inf.RoundUp)
	rDec.Mul(rDec, mib)
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

// Warn returns a warning string explaining that the ratio is not considered fair use
func (r Ratio) Warn(limit *resource.Quantity) string {
	// WARNING(glrf) Warnings MUST NOT contain newlines. K8s will simply drop your warning if you add newlines.
	w := fmt.Sprintf("Current memory to CPU ratio of %s/core in this namespace is below the fair use ratio", r.String())
	if limit != nil {
		w = fmt.Sprintf("%s of %s/core", w, limit)
	}
	w = fmt.Sprintf("%s. APPUiO Cloud bills CPU requests which exceed the fair use ratio.", w)
	w = fmt.Sprintf("%s See https://vs.hn/appuio-cloud-cpu-requests for instructions to adjust the requests.", w)
	return w
}
