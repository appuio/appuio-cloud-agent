package skipper

import "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

type Skipper interface {
	Skip(admission.Request) (bool, error)
}

// StaticSkipper is a Skipper that never/always skips.
type StaticSkipper struct {
	ShouldSkip bool
}

func (s StaticSkipper) Skip(_ admission.Request) (bool, error) {
	return s.ShouldSkip, nil
}
