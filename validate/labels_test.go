package validate_test

import (
	"testing"

	"github.com/appuio/appuio-cloud-agent/validate"
	"github.com/stretchr/testify/assert"
)

func Test_AllowedLabels_Add(t *testing.T) {
	v := validate.AllowedLabels{}

	assert.NoError(t, v.Add("my.ns.io/.+", "a|b|c"))

	const invalidRegex = "invalid("
	assert.Error(t, v.Add(invalidRegex, "valid"))
	assert.Error(t, v.Add("valid", invalidRegex))
}

func Test_AllowedLabels_Validate(t *testing.T) {
	v := validate.AllowedLabels{}

	assert.NoError(t, v.Validate(map[string]string{}))
	assert.Error(t, v.Validate(map[string]string{"a": "b"}))

	v.Add("my.ns.io/.+", "a|b|c")
	v.Add("my.other.ns.io/.+", "a|b|c")

	assert.NoError(t, v.Validate(map[string]string{}))
	assert.NoError(t, v.Validate(map[string]string{
		"my.ns.io/label1":       "a",
		"my.other.ns.io/label1": "a",
	}))

	assert.Error(t, v.Validate(map[string]string{
		"my.ns.io/label1": "a",
		"my.ns.io/label2": "x",
	}))

	assert.Error(t, v.Validate(map[string]string{
		"my.ns.io/label1": "a",
		"some.other.io":   "a",
	}))

	err := v.Validate(map[string]string{"some.other.io": "a", "some.other.io/2": "a"})
	assert.ErrorContains(t, err, "some.other.io=a")
	assert.ErrorContains(t, err, "some.other.io/2=a")
}
