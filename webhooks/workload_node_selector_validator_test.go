package webhooks

import (
	"context"
	"testing"

	"github.com/appuio/appuio-cloud-agent/skipper"
	"github.com/appuio/appuio-cloud-agent/validate"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_WorkloadNodeSelectorValidator_Handle(t *testing.T) {
	allowed := &validate.AllowedLabels{}
	require.NoError(t, allowed.Add("appuio.io/node-class", "flex|plus"))

	subject := WorkloadNodeSelectorValidator{
		AllowedNodeSelectors: allowed,
		Skipper:              skipper.StaticSkipper{},
	}
	require.NoError(t, subject.InjectDecoder(decoder(t)))

	allowedNodeSelector := map[string]string{"appuio.io/node-class": "flex"}

	testCases := []struct {
		name    string
		object  client.Object
		allowed bool
	}{
		{"no node selector", newDeployment("empty", nil), true},
		{"disallowed node selector", newDeployment("disallowed", map[string]string{"appuio.io/node-class": "premium"}), false},
		{"allowed node selector", newDeployment("allowed", allowedNodeSelector), true},
		{"allowed node selector", newCronJob("allowed", allowedNodeSelector), true},
		{"allowed node selector", newJob("allowed", allowedNodeSelector), true},
		{"allowed node selector", newDaemonSet("allowed", allowedNodeSelector), true},
		{"allowed node selector", newStatefulSet("allowed", allowedNodeSelector), true},
		{"allowed node selector", newPod("allowed", allowedNodeSelector), true},
		{"unknown object", newConfigMap("allowed"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := subject.Handle(context.Background(), admissionRequestForObject(t, tc.object))
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.Equal(t, tc.allowed, resp.Allowed)
		})
	}
}

func newPod(name string, nodeSelector map[string]string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PodSpec{
			NodeSelector: nodeSelector,
		},
	}
}

func newCronJob(name string, nodeSelector map[string]string) *batchv1.CronJob {
	return &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CronJob",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: nodeSelector,
						},
					},
				},
			},
		},
	}
}

func newJob(name string, nodeSelector map[string]string) *batchv1.Job {
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: nodeSelector,
				},
			},
		},
	}
}

func newDeployment(name string, nodeSelector map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: nodeSelector,
				},
			},
		},
	}
}

func newDaemonSet(name string, nodeSelector map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: nodeSelector,
				},
			},
		},
	}
}

func newStatefulSet(name string, nodeSelector map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: nodeSelector,
				},
			},
		},
	}
}

func newConfigMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
