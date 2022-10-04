package skipper

import "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

type Skipper interface {
	Skip(admission.Request) (bool, error)
}

// NoopSkipper is a Skipper that never/always skips.
type NoopSkipper struct {
	ShouldSkip bool
}

func (s NoopSkipper) Skip(_ admission.Request) (bool, error) {
	return s.ShouldSkip, nil
}
