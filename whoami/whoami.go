package whoami

import (
	"context"
	"fmt"
	"net/http"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authenticationv1cli "k8s.io/client-go/kubernetes/typed/authentication/v1"
	"k8s.io/client-go/rest"
)

// Whoami can return the current user from a self subject review.
type Whoami struct {
	Client authenticationv1cli.SelfSubjectReviewInterface
}

// WhoamiForConfigAndClient creates a new Whoami instance for the given config and client.
func WhoamiForConfigAndClient(c *rest.Config, h *http.Client) (*Whoami, error) {
	client, err := authenticationv1cli.NewForConfigAndClient(c, h)
	if err != nil {
		return nil, fmt.Errorf("error while creating self subject review client: %w", err)
	}
	return &Whoami{
		Client: client.SelfSubjectReviews(),
	}, nil
}

// Whoami returns the current user from a self subject review.
func (s *Whoami) Whoami(ctx context.Context) (authenticationv1.UserInfo, error) {
	ssr, err := s.Client.Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return authenticationv1.UserInfo{}, fmt.Errorf("error while creating self subject review: %w", err)
	}

	return ssr.Status.UserInfo, nil
}
