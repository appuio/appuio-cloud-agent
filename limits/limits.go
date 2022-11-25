package limits

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Limit struct {
	NodeSelector metav1.LabelSelector
	Limit        *resource.Quantity
}

type Limits []Limit

// GetLimitForNodeSelector returns the first limit that matches the given node selector.
// If no limit matches, an empty string is returned.
// Limits with invalid node selectors are ignored.
func (l Limits) GetLimitForNodeSelector(nodeSelector map[string]string) *resource.Quantity {
	for _, limit := range l {
		limitSel, err := metav1.LabelSelectorAsSelector(&limit.NodeSelector)
		if err != nil {
			continue
		}

		if limitSel.Matches(labels.Set(nodeSelector)) {
			return limit.Limit
		}
	}

	return nil
}
