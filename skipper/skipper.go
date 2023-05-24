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

type multiSkipper struct {
	skipper []Skipper
}

// NewMultiSkipper returns a Skipper that skips if any of the given Skipper skip.
func NewMultiSkipper(skipper ...Skipper) Skipper {
	return multiSkipper{skipper: skipper}
}

// Skip skips if any of the given Skipper skip.
func (s multiSkipper) Skip(ctx context.Context, req admission.Request) (bool, error) {
	for _, skipper := range s.skipper {
		skip, err := skipper.Skip(ctx, req)
		if err != nil {
			return skip, err
		}
		if skip {
			return true, nil
		}
	}
	return false, nil
}
