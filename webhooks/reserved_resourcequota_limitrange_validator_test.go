package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/appuio/appuio-cloud-agent/skipper"
)

func Test_ReservedResourceQuotaLimitRangeValidator_Handle(t *testing.T) {
	t.Parallel()

	_, scheme, decoder := prepareClient(t)

	subject := ReservedResourceQuotaLimitRangeValidator{
		Decoder: decoder,
		Skipper: skipper.StaticSkipper{},

		ReservedResourceQuotaNames: []string{"org"},
		ReservedLimitRangeNames:    []string{"org"},
	}

	testCases := []struct {
		name    string
		subject client.Object
		allowed bool
	}{
		{
			name:    "LimitRange: reserved name",
			subject: &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "org"}},
			allowed: false,
		},
		{
			name:    "LimitRange: allowed name",
			subject: &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "not-org"}},
			allowed: true,
		},
		{
			name:    "ResourceQuota: reserved name",
			subject: &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "org"}},
			allowed: false,
		},
		{
			name:    "ResourceQuota: allowed name",
			subject: &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "not-org"}},
			allowed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := subject.Handle(context.Background(), admissionRequestForObject(t, tc.subject, scheme))
			t.Log("Response:", resp.Result.Reason, resp.Result.Message)
			require.Equal(t, tc.allowed, resp.Allowed)
		})
	}
}
