package webhooks

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestRatio_recordPod(t *testing.T) {
	tcs := map[string]struct {
		podResources []podResource
		cpuSum       string
		memorySum    string
	}{
		"single container": {
			podResources: []podResource{
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
			podResources: []podResource{
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
			podResources: []podResource{
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
	}

	for k, tc := range tcs {
		t.Run(k, func(t *testing.T) {
			var pods []corev1.Pod

			for _, pr := range tc.podResources {
				pod := corev1.Pod{}
				for _, cr := range pr {
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
					pod.Spec.Containers = append(pod.Spec.Containers, container)
				}
				pods = append(pods, pod)
			}

			r := &ratio{}
			r.recordPod(pods...)

			assertResourceEqual(t, r.cpu, tc.cpuSum)
			assertResourceEqual(t, r.memory, tc.memorySum)
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
			ratio:           fmt.Sprintf("%d", math.MaxInt64),
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
			r := ratio{
				cpu:    &cpu,
				memory: &memory,
			}
			assertResourceEqual(t, r.ratio(), tc.ratio)
			if tc.smallerThan != "" {
				assert.Truef(t, r.below(resource.MustParse(tc.smallerThan)), "should be smaller than %s", tc.smallerThan)
			}
			if tc.largerOrEqualTo != "" {
				assert.Falsef(t, r.below(resource.MustParse(tc.largerOrEqualTo)), "should not be smaller than %s", tc.largerOrEqualTo)
			}
			assert.Equalf(t, tc.ratio, r.String(), "should pretty print")
		})
	}
}

func assertResourceEqual(t *testing.T, res *resource.Quantity, s string) bool {
	return assert.Truef(t, res.Equal(resource.MustParse(s)), "%s should be equal to %s", res, s)
}

type podResource []containerResources
type containerResources struct {
	cpu    string
	memory string
}
