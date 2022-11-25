package limits_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/appuio/appuio-cloud-agent/limits"
)

func TestGetLimitForNodeSelector(t *testing.T) {
	subject := limits.Limits{
		{
			NodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "class",
						Operator: "Invalid",
					},
				},
			},
			Limit: requireParseQuantity(t, "7Gi"),
		},
		{
			NodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "class",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
				},
			},
			Limit: requireParseQuantity(t, "7Gi"),
		},
		{
			NodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "class",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"highmem"},
					},
				},
			},
			Limit: requireParseQuantity(t, "14Gi"),
		},
		{
			NodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "gpu",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"tesla"},
					},
				},
			},
			Limit: requireParseQuantity(t, "2Gi"),
		},
	}

	assert.Equal(t, "7Gi", subject.GetLimitForNodeSelector(map[string]string{}).String())
	assert.Equal(t, "7Gi", subject.GetLimitForNodeSelector(map[string]string{"blubber": "blubber"}).String())

	assert.Equal(t, "14Gi", subject.GetLimitForNodeSelector(map[string]string{"class": "highmem"}).String())
	assert.Equal(t, "14Gi", subject.GetLimitForNodeSelector(map[string]string{"class": "highmem", "blubber": "blubber"}).String())
	// First match wins
	assert.Equal(t, "14Gi", subject.GetLimitForNodeSelector(map[string]string{"class": "highmem", "gpu": "tesla"}).String())

	assert.Equal(t, "2Gi", subject.GetLimitForNodeSelector(map[string]string{"class": "other", "gpu": "tesla"}).String())

	assert.Nil(t, subject.GetLimitForNodeSelector(map[string]string{"class": "other", "gpu": "other"}))
}

func requireParseQuantity(t *testing.T, s string) *resource.Quantity {
	t.Helper()
	q, err := resource.ParseQuantity(s)
	require.NoError(t, err)
	return &q
}
