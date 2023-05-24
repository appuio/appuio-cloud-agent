package skipper_test

import (
	"context"
	"errors"
	"testing"

	"github.com/appuio/appuio-cloud-agent/skipper"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func Test_MultiSkipper(t *testing.T) {
	{
		skipper := skipper.NewMultiSkipper(
			&skipper.StaticSkipper{ShouldSkip: false},
			&skipper.StaticSkipper{ShouldSkip: true},
			&skipper.StaticSkipper{ShouldSkip: false},
		)

		skip, err := skipper.Skip(context.Background(), admission.Request{})
		require.NoError(t, err)
		require.True(t, skip)
	}

	{
		skipper := skipper.NewMultiSkipper(
			&skipper.StaticSkipper{ShouldSkip: false},
			&skipper.StaticSkipper{ShouldSkip: false},
		)

		skip, err := skipper.Skip(context.Background(), admission.Request{})
		require.NoError(t, err)
		require.False(t, skip)
	}

	{
		expectedErr := errors.New("some error")
		skipper := skipper.NewMultiSkipper(errorSkipper{Error: expectedErr})

		_, err := skipper.Skip(context.Background(), admission.Request{})
		require.Equal(t, expectedErr, err)
	}
}

type errorSkipper struct{ Error error }

func (e errorSkipper) Skip(_ context.Context, _ admission.Request) (bool, error) {
	return false, e.Error
}
