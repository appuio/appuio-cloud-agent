package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

func Test_PodNodeSelectorMutator_Handle(t *testing.T) {

	crs := []client.Object{
		newNamespace("ns-with-default-label", nil, map[string]string{AppuioIoDefaultNodeSelector: "appuio.io/node-class=bar"}),
		newNamespace("ns-with-empty-label", nil, map[string]string{AppuioIoDefaultNodeSelector: ""}),
		newNamespace("ns-no-annotations", nil, nil),
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crs...).Build()

	testCases := []struct {
		name string

		defaultNodeSelector map[string]string

		namespace    string
		nodeSelector map[string]string
		allowed      bool
		patch        []jsonpatch.Operation
	}{
		{
			"node selector map exists, ns with default label",
			nil,
			"ns-with-default-label",
			map[string]string{"other-sel": "foo"},
			true,
			[]jsonpatch.Operation{jsonpatch.NewOperation("add", "/spec/nodeSelector/appuio.io~1node-class", "bar")},
		},
		{
			"no node selector, ns with default label",
			labels.Set{"appuio.io/node-class": "bar"},
			"ns-with-empty-label",
			nil,
			true,
			[]jsonpatch.Operation{jsonpatch.NewOperation("add", "/spec/nodeSelector", labels.Set{"appuio.io/node-class": "bar"})},
		},
		{
			"no node selector, default from config",
			nil,
			"ns-with-default-label",
			nil,
			true,
			[]jsonpatch.Operation{jsonpatch.NewOperation("add", "/spec/nodeSelector", labels.Set{"appuio.io/node-class": "bar"})},
		},
		{
			"node selector, ns with default label - should not override",
			nil,
			"ns-with-default-label",
			map[string]string{"appuio.io/node-class": "foo"},
			true,
			[]jsonpatch.Operation{},
		},
		{
			"no node selector, ns without default label",
			nil,
			"ns-with-empty-label",
			nil,
			true,
			[]jsonpatch.Operation{},
		},
		{
			"no node selector, ns without default label",
			nil,
			"ns-no-annotations",
			nil,
			true,
			[]jsonpatch.Operation{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			subject := PodNodeSelectorMutator{
				Client:              c,
				Skipper:             skipper.StaticSkipper{},
				DefaultNodeSelector: tc.defaultNodeSelector,
			}
			subject.InjectDecoder(decoder(t))

			pod := newPod(tc.namespace, "test", tc.nodeSelector)
			resp := subject.Handle(context.Background(), admissionRequestForObject(t, pod))
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.ElementsMatch(t, tc.patch, resp.Patches)
			require.Equal(t, tc.allowed, resp.Allowed)
		})
	}
}
