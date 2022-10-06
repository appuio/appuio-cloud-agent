package skipper

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Skipper interface {
	Skip(context.Context, admission.Request) (bool, error)
}

var _ Skipper = StaticSkipper{}

// StaticSkipper is a Skipper that never/always skips.
type StaticSkipper struct {
	ShouldSkip bool
}

func (s StaticSkipper) Skip(_ context.Context, _ admission.Request) (bool, error) {
	return s.ShouldSkip, nil
}
