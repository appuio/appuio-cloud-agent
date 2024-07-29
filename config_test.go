package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/inf.v0"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const limitsYAML = `
MemoryPerCoreLimits:
- NodeSelector:
    matchExpressions:
      - key: class
        operator: DoesNotExist
  Limit: 2Gi
`
const limitYAML = `
MemoryPerCoreLimit: 1Gi
`

func Test_Config_MemoryPerCoreLimits(t *testing.T) {
	testCases := []struct {
		desc         string
		yaml         string
		warnings     int
		limit        string
		nodeSelector metav1.LabelSelector
	}{
		{
			desc:     "MemoryPerCoreLimits",
			yaml:     limitsYAML,
			warnings: 0,
			limit:    "2Gi",
			nodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "class",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
				},
			},
		},
		{
			desc:         "MemoryPerCoreLimit_Migrate",
			yaml:         limitYAML,
			warnings:     1,
			limit:        "1Gi",
			nodeSelector: metav1.LabelSelector{},
		},
		{
			desc:     "MemoryPerCoreLimit_NoMigrateIfMemoryPerCoreLimitsIsSet",
			yaml:     strings.Join([]string{limitsYAML, limitYAML}, "\n"),
			warnings: 0,
			limit:    "2Gi",
			nodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "class",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
				},
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			tmp := t.TempDir()
			configPath := filepath.Join(tmp, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tC.yaml), 0o644))

			c, warnings, err := ConfigFromFile(configPath)
			assert.Len(t, warnings, tC.warnings)
			require.NoError(t, err)

			assert.Equal(t, tC.limit, c.MemoryPerCoreLimits[0].Limit.String())
			assert.Equal(t, tC.nodeSelector, c.MemoryPerCoreLimits[0].NodeSelector)
		})
	}
}

func Test_Config_MemoryPerCoreWarnThreshold(t *testing.T) {
	tc := []struct {
		yaml     string
		expected *inf.Dec
	}{
		{
			yaml:     `MemoryPerCoreWarnThreshold: "0.95"`,
			expected: inf.NewDec(95, 2),
		}, {
			expected: nil,
		},
	}

	for _, tC := range tc {
		t.Run(tC.yaml, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			configPath := filepath.Join(tmp, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tC.yaml), 0o644))

			c, warnings, err := ConfigFromFile(configPath)
			assert.Len(t, warnings, 0)
			require.NoError(t, err)

			assert.Equal(t, tC.expected, c.MemoryPerCoreWarnThreshold)
		})
	}
}
