package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func Test_Config_MemoryPerCoreLimit(t *testing.T) {
	raw := `
MemoryPerCoreLimits:
- NodeSelector:
    matchExpressions:
      - key: class
        operator: DoesNotExist
  Limit: 7Gi
`

	var c Config
	err := yaml.Unmarshal([]byte(raw), &c, yaml.DisallowUnknownFields)
	require.NoError(t, err)

	assert.Equal(t, "7Gi", c.MemoryPerCoreLimits[0].Limit.String())
	assert.Equal(t, metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "class",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	}, c.MemoryPerCoreLimits[0].NodeSelector)
}
