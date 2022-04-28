package ratio

import (
	"context"
	"errors"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RatioValidatiorDisableAnnotation is the key for an annotion on a namespace to disable request ratio warnings
var RatioValidatiorDisableAnnotation = "validate-request-ratio.appuio.io/disable"

// ErrorDisabled is returned if the request ratio validation is disabled
var ErrorDisabled error = errors.New("request ratio validation disabled")

// Fetcher collects the CPU to memory request ratio
type Fetcher struct {
	Client client.Client

	OrganizationLabel string
}

// FetchRatio collects the CPU to memory request ratio for the given namespace
func (f Fetcher) FetchRatio(ctx context.Context, name string) (*Ratio, error) {
	ns := corev1.Namespace{}
	err := f.Client.Get(ctx, client.ObjectKey{
		Name: name,
	}, &ns)
	if err != nil {
		return nil, err
	}

	disabledAnnot, ok := ns.Annotations[RatioValidatiorDisableAnnotation]
	if ok {
		disabled, err := strconv.ParseBool(disabledAnnot)
		if err != nil || disabled {
			return nil, ErrorDisabled
		}
	}

	if f.OrganizationLabel != "" {
		if _, isOrgNs := ns.Labels[f.OrganizationLabel]; !isOrgNs {
			return nil, ErrorDisabled
		}
	}

	r := NewRatio()
	pods := corev1.PodList{}
	err = f.Client.List(ctx, &pods, client.InNamespace(name))
	if err != nil {
		return r, err
	}
	return r.RecordPod(pods.Items...), nil
}
