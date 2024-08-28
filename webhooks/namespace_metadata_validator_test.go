package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

func Test_NamespaceMetadataValidator_Handle(t *testing.T) {

	testCases := []struct {
		name string

		reservedNamespaces []string
		allowedAnnotations []string
		allowedLabels      []string

		object client.Object
		oldObj client.Object

		allowed bool
	}{
		{
			name:   "new namespace with allowed name",
			object: newNamespace("test-namespace", nil, nil),

			reservedNamespaces: []string{"appuio*"},

			allowed: true,
		},
		{
			name:   "new project with allowed name",
			object: newProjectRequest("test-project", nil, nil),

			reservedNamespaces: []string{"appuio*"},

			allowed: true,
		},
		{
			name:   "new namespace with reserved name",
			object: newNamespace("appuio-blub", nil, nil),

			reservedNamespaces: []string{"test", "appuio*"},

			allowed: false,
		},
		{
			name:   "new project with reserved name",
			object: newProjectRequest("appuio-blub", nil, nil),

			reservedNamespaces: []string{"test", "appuio*"},

			allowed: false,
		},
		{
			name:   "new namespace with allowed annotation",
			object: newNamespace("test-namespace", nil, map[string]string{"allowed": ""}),

			allowedAnnotations: []string{"allowed"},
			allowed:            true,
		},
		{
			name:   "new namespace with disallowed annotation",
			object: newNamespace("test-namespace", nil, map[string]string{"disallowed": ""}),

			allowedAnnotations: []string{"allowed"},
			allowed:            false,
		},
		{
			name:   "new namespace with allowed label",
			object: newNamespace("test-namespace", map[string]string{"allowed-kajshd": "", "custom/x": "asd"}, nil),

			allowedLabels: []string{"allowed*", "custom/*"},
			allowed:       true,
		},
		{
			name:   "new namespace with disallowed label",
			object: newNamespace("test-namespace", map[string]string{"disallowed": ""}, nil),

			allowedLabels: []string{"allowed"},
			allowed:       false,
		},
		{
			name:   "update namespace with allowed annotation",
			object: newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s", "allowed": ""}),
			oldObj: newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s"}),

			allowedAnnotations: []string{"allowed"},
			allowed:            true,
		},
		{
			name:               "update namespace with disallowed annotation",
			object:             newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s", "disallowed": "a"}),
			oldObj:             newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s", "disallowed": "b"}),
			allowedAnnotations: []string{"allowed"},
			allowed:            false,
		},
		{
			name:               "remove disallowed annotation",
			object:             newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s"}),
			oldObj:             newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s", "disallowed": "b", "disallowed2": "", "allowed": ""}),
			allowedAnnotations: []string{"allowed"},
			allowed:            false,
		},
		{
			name:               "remove disallowed annotation",
			object:             newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s"}),
			oldObj:             newNamespace("test-namespace", nil, map[string]string{"pre-existing": "s", "disallowed": ""}),
			allowedAnnotations: []string{"allowed"},
			allowed:            false,
		},
	}

	_, scheme, decoder := prepareClient(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			subject := &NamespaceMetadataValidator{
				Decoder:            decoder,
				Skipper:            skipper.StaticSkipper{},
				ReservedNamespaces: tc.reservedNamespaces,
				AllowedAnnotations: tc.allowedAnnotations,
				AllowedLabels:      tc.allowedLabels,
			}

			amr := admissionRequestForObjectWithOldObject(t, tc.object, tc.oldObj, scheme)

			resp := subject.Handle(context.Background(), amr)
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.Equal(t, tc.allowed, resp.Allowed)
		})
	}
}
