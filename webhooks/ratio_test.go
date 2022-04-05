package webhooks

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
					{
						cpu:    "1",
						memory: "4Gi",
					},
				},
			},
			cpuSum:    "1",
			memorySum: "4Gi",
		},
		"multi container": {
			pods: []podResource{
				{
					{
						cpu:    "500m",
						memory: "4Gi",
					},
					{
						cpu:    "701m",
						memory: "1Gi",
					},
				},
			},
			cpuSum:    "1201m",
			memorySum: "5Gi",
		},
		"multi pod": {
			pods: []podResource{
				{
					{
						cpu:    "500m",
						memory: "4Gi",
					},
					{
						cpu:    "101m",
						memory: "1Gi",
					},
				},
				{
					{
						memory: "1Gi",
					},
					{
						cpu:    "101m",
						memory: "101Mi",
					},
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
			deployments: []deployResource{
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
				pod := corev1.Pod{}
				pod.Spec.Containers = newTestContainers(pr)
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
				sts.Spec.Replicas = &tc.deployments[i].replicas
				sts.Spec.Template.Spec.Containers = newTestContainers(tc.deployments[i].containers)
				r.RecordStatefulSet(sts)
			}

			assertResourceEqual(t, resource.NewDecimalQuantity(*r.cpu, resource.BinarySI), tc.cpuSum)
			assertResourceEqual(t, resource.NewDecimalQuantity(*r.memory, resource.BinarySI), tc.memorySum)
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
				cpu:    cpu.AsDec(),
				memory: memory.AsDec(),
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