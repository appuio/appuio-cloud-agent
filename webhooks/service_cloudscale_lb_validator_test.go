package webhooks

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

func Test_ServiceCloudscaleLBValidator_Handle(t *testing.T) {
	ctx := log.IntoContext(context.Background(), testr.New(t))

	tests := map[string]struct {
		object       client.Object
		oldObject    client.Object
		allowed      bool
		skip         bool
		matchMessage string
	}{
		"Create allow no annotations": {
			object:  newService("test", nil, nil),
			allowed: true,
		},
		"Create allow other annotation": {
			object:  newService("test", nil, map[string]string{"other": "value"}),
			allowed: true,
		},
		"Create deny lb annotation": {
			object:       newService("test", nil, map[string]string{CloudscaleLoadbalancerUUIDAnnotation: "value"}),
			allowed:      false,
			matchMessage: "k8s.cloudscale.ch/loadbalancer-uuid annotation cannot be changed",
		},

		"Create allow skipped": {
			object:       newService("test", nil, map[string]string{CloudscaleLoadbalancerUUIDAnnotation: "value"}),
			allowed:      true,
			skip:         true,
			matchMessage: "skipped",
		},

		"Update allow no annotations": {
			object:    newService("test", nil, nil),
			oldObject: newService("test", nil, nil),
			allowed:   true,
		},
		"Update allow other annotation": {
			object:    newService("test", nil, map[string]string{"other": "value"}),
			oldObject: newService("test", nil, map[string]string{"other": "value2"}),
			allowed:   true,
		},
		"Update allow new other annotation": {
			object:    newService("test", nil, map[string]string{"other": "value"}),
			oldObject: newService("test", nil, nil),
			allowed:   true,
		},
		"Update allow delete other annotation": {
			object:    newService("test", nil, nil),
			oldObject: newService("test", nil, map[string]string{"other": "value"}),
			allowed:   true,
		},
		"Update deny add lb annotation": {
			object:       newService("test", nil, map[string]string{CloudscaleLoadbalancerUUIDAnnotation: "value"}),
			oldObject:    newService("test", nil, nil),
			allowed:      false,
			matchMessage: "k8s.cloudscale.ch/loadbalancer-uuid annotation cannot be changed",
		},
		"Update deny update lb annotation": {
			object:       newService("test", nil, map[string]string{CloudscaleLoadbalancerUUIDAnnotation: "value2"}),
			oldObject:    newService("test", nil, map[string]string{CloudscaleLoadbalancerUUIDAnnotation: "value"}),
			allowed:      false,
			matchMessage: "k8s.cloudscale.ch/loadbalancer-uuid annotation cannot be changed",
		},
	}

	_, scheme, dec := prepareClient(t)
	for name, tC := range tests {
		t.Run(name, func(t *testing.T) {
			subject := &ServiceCloudscaleLBValidator{
				Decoder: dec,
				Skipper: skipper.StaticSkipper{ShouldSkip: tC.skip},
			}
			res := subject.Handle(ctx, admissionRequestForObjectWithOldObject(t, tC.object, tC.oldObject, scheme))
			require.Equal(t, tC.allowed, res.Allowed)
			if tC.matchMessage != "" {
				require.Contains(t, res.Result.Message, tC.matchMessage)
			}
		})
	}
}
