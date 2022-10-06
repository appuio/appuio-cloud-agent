package validate

import (
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/multierr"
)

type regexKV struct {
	K *regexp.Regexp
	V *regexp.Regexp
}

func (r regexKV) String() string {
	return fmt.Sprintf("%s=%s", r.K, r.V)
}

// AllowedLabels allows labels to be validated against a set of allowed labels.
// The zero value is ready to use and denies all labels.
type AllowedLabels struct {
	allowed []regexKV
}

// Add adds a new allowed label.
// The key and value are anchored regular expressions.
// An error is returned if the regular expressions are invalid or the key is empty.
func (l *AllowedLabels) Add(key, value string) error {
	if l.allowed == nil {
		l.allowed = make([]regexKV, 0)
	}

	keyR, err := regexp.Compile(anchor(key))
	if err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}
	valueR, err := regexp.Compile(anchor(value))
	if err != nil {
		return fmt.Errorf("invalid value: %w", err)
	}

	l.allowed = append(l.allowed, regexKV{K: keyR, V: valueR})
	return nil
}

// Validate validates all labels against the allowed labels.
func (l *AllowedLabels) Validate(lbls map[string]string) error {
	violations := make([]error, 0, len(lbls))
	for k, v := range lbls {
		violations = append(violations, l.ValidateLabel(k, v))
	}

	if err := multierr.Combine(violations...); err != nil {
		return fmt.Errorf("label validation failed: %w, allowed labels: %s", err, l.formatLabels())
	}

	return nil
}

func anchor(s string) string {
	return "^(?:" + s + ")$"
}

// ValidateLabel validates a single label against the allowed labels.
func (l *AllowedLabels) ValidateLabel(key, value string) error {
	for _, allowed := range l.allowed {
		if allowed.K.MatchString(key) && allowed.V.MatchString(value) {
			return nil
		}
	}

	return fmt.Errorf("label %s=%s is not allowed", key, value)
}

func (l *AllowedLabels) String() string {
	return fmt.Sprintf("allowed %s", l.formatLabels())
}

func (l *AllowedLabels) formatLabels() string {
	s := make([]string, 0, len(l.allowed))
	for _, allowed := range l.allowed {
		s = append(s, allowed.String())
	}

	return fmt.Sprintf("{ %s }", strings.Join(s, ", "))
}
