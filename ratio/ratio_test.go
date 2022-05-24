package ratio

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestRatio_Record(t *testing.T) {
	tcs := map[string]struct {
		pods         []podResource
		deployments  []deployResource
		statefulsets []deployResource
		cpuSum       string
		memorySum    string
	}{
		"single container": {
			pods: []podResource{
				{
					containers: []containerResources{
						{
							cpu:    "1",
							memory: "4Gi",
						},
					},
					phase: corev1.PodRunning,
				},
			},
			cpuSum:    "1",
			memorySum: "4Gi",
		},
		"multi container": {
			pods: []podResource{
				{
					containers: []containerResources{
						{
							cpu:    "500m",
							memory: "4Gi",
						},
						{
							cpu:    "701m",
							memory: "1Gi",
						},
					},
					phase: corev1.PodRunning,
				},
			},
			cpuSum:    "1201m",
			memorySum: "5Gi",
		},
		"multi pod": {
			pods: []podResource{
				{
					containers: []containerResources{
						{
							cpu:    "500m",
							memory: "4Gi",
						},
						{
							cpu:    "101m",
							memory: "1Gi",
						},
					},
					phase: corev1.PodRunning,
				},
				{
					containers: []containerResources{
						{
							memory: "1Gi",
						},
						{
							cpu:    "101m",
							memory: "101Mi",
						},
					},
					phase: corev1.PodRunning,
				},
			},
			cpuSum:    "702m",
			memorySum: "6245Mi",
		},
		"deployments": {
			deployments: []deployResource{
				{
					replicas: 4,
					containers: []containerResources{
						{
							cpu:    "500m",
							memory: "4Gi",
						},
						{
							cpu:    "101m",
							memory: "1Gi",
						},
					},
				},
				{
					replicas: 2,
					containers: []containerResources{
						{
							cpu:    "250m",
							memory: "3Gi",
						},
					},
				},
			},
			cpuSum:    "2904m",
			memorySum: "26Gi",
		},
		"statefulsets": {
			statefulsets: []deployResource{
				{
					replicas: 4,
					containers: []containerResources{
						{
							cpu:    "400m",
							memory: "3Gi",
						},
						{
							cpu:    "101m",
							memory: "10Mi",
						},
					},
				},
				{
					replicas: 2,
					containers: []containerResources{
						{
							cpu:    "250m",
							memory: "3Gi",
						},
					},
				},
			},
			cpuSum:    "2504m",
			memorySum: "18472Mi",
		},
	}

	for k, tc := range tcs {
		t.Run(k, func(t *testing.T) {

			r := NewRatio()
			for _, pr := range tc.pods {
				pod := corev1.Pod{
					Status: corev1.PodStatus{
						Phase: pr.phase,
					},
				}
				pod.Spec.Containers = newTestContainers(pr.containers)
				r.RecordPod(pod)
			}

			for i := range tc.deployments {
				deploy := appsv1.Deployment{}
				deploy.Spec.Replicas = &tc.deployments[i].replicas
				deploy.Spec.Template.Spec.Containers = newTestContainers(tc.deployments[i].containers)
				r.RecordDeployment(deploy)
			}
			for i := range tc.statefulsets {
				sts := appsv1.StatefulSet{}
				sts.Spec.Replicas = &tc.statefulsets[i].replicas
				sts.Spec.Template.Spec.Containers = newTestContainers(tc.statefulsets[i].containers)
				r.RecordStatefulSet(sts)
			}

			assertResourceEqual(t, resource.NewDecimalQuantity(*r.CPU, resource.BinarySI), tc.cpuSum)
			assertResourceEqual(t, resource.NewDecimalQuantity(*r.Memory, resource.BinarySI), tc.memorySum)
		})
	}
}

func TestRatio_ratio(t *testing.T) {
	tcs := []struct {
		cpu    string
		memory string
		ratio  string

		smallerThan     string
		largerOrEqualTo string
	}{
		{
			cpu:         "1",
			memory:      "4Gi",
			ratio:       "4Gi",
			smallerThan: "5Gi",
		},
		{
			cpu:             "2",
			memory:          "4Gi",
			ratio:           "2Gi",
			largerOrEqualTo: "1Gi",
		},
		{
			cpu:    "2",
			memory: "5Gi",
			ratio:  "2560Mi",
		},
		{
			memory:          "5Gi",
			largerOrEqualTo: "2500Gi",
		},
		{
			cpu:         "2",
			ratio:       "0",
			smallerThan: "1Mi",
		},
		{
			// Ratio gets rounded up, exact result would be 1917396114 bytes
			cpu:    "1.4",
			memory: "2560Mi",
			ratio:  "1829Mi",
		},
		{
			// Ratio gets rounded up, exact result would be 400.5Mi
			cpu:    "2",
			memory: "801Mi",
			ratio:  "401Mi",
		},
	}
	for _, tc := range tcs {
		t.Run(fmt.Sprintf("[%s/%s=%s]", tc.memory, tc.cpu, tc.ratio), func(t *testing.T) {
			var cpu resource.Quantity
			var memory resource.Quantity
			if tc.cpu != "" {
				cpu = resource.MustParse(tc.cpu)
			}
			if tc.memory != "" {
				memory = resource.MustParse(tc.memory)
			}
			r := Ratio{
				CPU:    cpu.AsDec(),
				Memory: memory.AsDec(),
			}
			if tc.ratio != "" {
				assertResourceEqual(t, r.Ratio(), tc.ratio)
				assert.Equalf(t, tc.ratio, r.String(), "should pretty print")
			}
			if tc.smallerThan != "" {
				assert.Truef(t, r.Below(resource.MustParse(tc.smallerThan)), "should be smaller than %s", tc.smallerThan)
			}
			if tc.largerOrEqualTo != "" {
				assert.Falsef(t, r.Below(resource.MustParse(tc.largerOrEqualTo)), "should not be smaller than %s", tc.largerOrEqualTo)
			}
		})
	}
}

func TestRatio_Warn(t *testing.T) {
	cpu := resource.MustParse("1")
	memory := resource.MustParse("1024Mi")
	r := Ratio{
		CPU:    cpu.AsDec(),
		Memory: memory.AsDec(),
	}
	assert.Contains(t, r.Warn(nil), "1Gi")
	lim := resource.MustParse("1Mi")
	assert.Contains(t, r.Warn(&lim), "1Mi")
}

func FuzzRatio(f *testing.F) {
	f.Add(1, 1024, 512)
	f.Fuzz(func(t *testing.T, cpu int, memory int, limit int) {
		assert.NotPanics(t, func() {
			t.Logf("Input: \n\tCPU: %d, Memory: %d, Limit: %d", cpu, memory, limit)
			cpuQuant := resource.MustParse(fmt.Sprintf("%dm", cpu))
			memQuant := resource.MustParse(fmt.Sprintf("%dMi", memory))
			r := Ratio{
				CPU:    cpuQuant.AsDec(),
				Memory: memQuant.AsDec(),
			}
			lim := resource.MustParse(fmt.Sprintf("%dMi", limit))
			out := r.Warn(&lim)
			assert.NotEmpty(t, out)

			r.Below(lim)
		})
	})
}
