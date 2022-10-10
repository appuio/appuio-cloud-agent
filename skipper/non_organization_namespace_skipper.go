package skipper

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NonOrganizationNamespaceSkipper skips requests for namespaces that don't have the organization label.
type NonOrganizationNamespaceSkipper struct {
	OrganizationLabel string
	Client            client.Reader
}

var _ Skipper = &NonOrganizationNamespaceSkipper{}

// Skip skips requests for namespaces that don't have the organization label.
func (s *NonOrganizationNamespaceSkipper) Skip(ctx context.Context, req admission.Request) (bool, error) {
	var ns corev1.Namespace
	if err := s.Client.Get(ctx, client.ObjectKey{Name: req.Namespace}, &ns); err != nil {
		return false, fmt.Errorf("error while fetching namespace: %w", err)
	}

	return ns.Labels[s.OrganizationLabel] == "", nil
}
