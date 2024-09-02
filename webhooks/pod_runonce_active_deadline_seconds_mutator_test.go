package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

func Test_PodRunOnceActiveDeadlineSecondsMutator_Handle(t *testing.T) {
	const overrideAnnotation = "appuio.io/active-deadline-seconds-override"
	const defaultActiveDeadlineSeconds = 60

	testCases := []struct {
		name string

		subject           client.Object
		additionalObjects []client.Object

		allowed                       bool
		expectedActiveDeadlineSeconds int
	}{
		{
			name: "pod with restartPolicy=Always",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
			}),
			additionalObjects: []client.Object{
				newNamespace("testns", nil, nil),
			},
			allowed: true,
		},
		{
			name: "pod with restartPolicy=OnFailure",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyOnFailure,
			}),
			additionalObjects: []client.Object{
				newNamespace("testns", nil, nil),
			},
			allowed:                       true,
			expectedActiveDeadlineSeconds: defaultActiveDeadlineSeconds,
		},
		{
			name: "pod with restartPolicy=Never",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
			}),
			additionalObjects: []client.Object{
				newNamespace("testns", nil, nil),
			},
			allowed:                       true,
			expectedActiveDeadlineSeconds: defaultActiveDeadlineSeconds,
		},
		{
			name: "pod in namespace with override annotation",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
			}),
			additionalObjects: []client.Object{
				newNamespace("testns", nil, map[string]string{
					overrideAnnotation: "30",
				}),
			},
			allowed:                       true,
			expectedActiveDeadlineSeconds: 30,
		},
		{
			name: "pod with existing activeDeadlineSeconds",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy:         corev1.RestartPolicyNever,
				ActiveDeadlineSeconds: ptr.To(int64(77)),
			}),
			additionalObjects: []client.Object{
				newNamespace("testns", nil, nil),
			},
			allowed: true,
		},
		{
			name: "pod in namespace with invalid override annotation",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
			}),
			additionalObjects: []client.Object{
				newNamespace("testns", nil, map[string]string{
					overrideAnnotation: "invalid",
				}),
			},
			allowed: false,
		},
		{
			name: "non-existing namespace",
			subject: newPodWithSpec("testns", "pod1", corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
			}),
			allowed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, scheme, decoder := prepareClient(t, tc.additionalObjects...)

			subject := PodRunOnceActiveDeadlineSecondsMutator{
				Decoder: decoder,
				Client:  c,
				Skipper: skipper.StaticSkipper{},

				OverrideAnnotation:           overrideAnnotation,
				DefaultActiveDeadlineSeconds: defaultActiveDeadlineSeconds,
			}

			resp := subject.Handle(context.Background(), admissionRequestForObject(t, tc.subject, scheme))
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.Equal(t, tc.allowed, resp.Allowed)

			if tc.expectedActiveDeadlineSeconds == 0 {
				require.Len(t, resp.Patches, 0)
				return
			}

			require.Len(t, resp.Patches, 1)
			require.Equal(t, tc.expectedActiveDeadlineSeconds, resp.Patches[0].Value)
		})
	}
}
