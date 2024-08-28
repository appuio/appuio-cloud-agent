package main

import (
	"errors"
	"os"

	"github.com/appuio/appuio-cloud-agent/limits"
	"go.uber.org/multierr"
	"gopkg.in/inf.v0"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

type Config struct {
	// OrganizationLabel is the label used to mark namespaces to belong to an organization
	OrganizationLabel string

	// UserDefaultOrganizationAnnotation is the annotation the default organization setting for a user is stored in.
	UserDefaultOrganizationAnnotation string

	// QuotaOverrideNamespace is the namespace where the quota overrides for organizations are stored.
	QuotaOverrideNamespace string

	// MemoryPerCoreLimit is the fair use limit of memory usage per CPU core
	// it is deprecated and will be removed in a future version.
	// Use MemoryPerCoreLimits: {Limit: "XGi"} instead.
	MemoryPerCoreLimit *resource.Quantity
	// MemoryPerCoreLimits is the fair use limit of memory usage per CPU core
	// It is possible to select limits by node selector labels
	MemoryPerCoreLimits limits.Limits
	// MemoryPerCoreWarnThreshold is the threshold at which a warning is emitted if the memory per core limit is exceeded.
	// Should be a decimal number resembling a percentage (e.g. "0.8" for 80%), represented as a string.
	// The limit is multiplied by the optional threshold before comparison.
	// A threshold of 0.95 would mean that the warnings are emitted if the ratio is below 95% of the limit.
	// Thus adding a leniency of 5% to the limit.
	MemoryPerCoreWarnThreshold *inf.Dec

	// Privileged* is a list of the given type allowed to bypass restrictions.
	// Wildcards are supported (e.g. "system:serviceaccount:default:*" or "cluster-*-operator").
	// ClusterRoles are only ever matched if they are bound through a ClusterRoleBinding,
	// this is different from the behavior of Kyverno.
	// This is done to prevent a user from wrongly configuring a low-privileged ClusterRole which users
	// can then bind to themselves to bypass the restrictions.
	PrivilegedGroups       []string
	PrivilegedUsers        []string
	PrivilegedClusterRoles []string

	// DefaultNodeSelector are the default node selectors to add to pods if not set from namespace annotation
	DefaultNodeSelector map[string]string
	// DefaultNamespaceNodeSelectorAnnotation is the annotation used to set the default node selector for pods in this namespace
	DefaultNamespaceNodeSelectorAnnotation string

	// DefaultOrganizationClusterRoles is a map containing the configuration for rolebindings that are created by default in each organization namespace.
	// The keys are the name of default rolebindings to create and the values are the names of the clusterroles they bind to.
	DefaultOrganizationClusterRoles map[string]string

	// ReservedNamespaces is a list of namespaces that are reserved and can't be created by users.
	// Supports '*' and '?' wildcards.
	ReservedNamespaces []string
	// AllowedAnnotations is a list of annotations that are allowed on namespaces.
	// Supports '*' and '?' wildcards.
	AllowedAnnotations []string
	// AllowedLabels is a list of labels that are allowed on namespaces.
	// Supports '*' and '?' wildcards.
	AllowedLabels []string
}

func ConfigFromFile(path string) (c Config, warn []string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, nil, err
	}
	err = yaml.Unmarshal(raw, &c, yaml.DisallowUnknownFields)
	if err != nil {
		return Config{}, nil, err
	}

	c, warnings := migrateConfig(c)
	return c, warnings, nil
}

func (c Config) Validate() error {
	var errs []error

	if c.OrganizationLabel == "" {
		errs = append(errs, errors.New("OrganizationLabel must not be empty"))
	}

	return multierr.Combine(errs...)
}

func migrateConfig(c Config) (Config, []string) {
	warnings := make([]string, 0)

	if c.MemoryPerCoreLimit != nil && c.MemoryPerCoreLimits == nil {
		warnings = append(warnings, "MemoryPerCoreLimit is deprecated and will be removed in a future version. Use MemoryPerCoreLimits: {Limit: \"XGi\"} instead.")
		c.MemoryPerCoreLimits = limits.Limits{
			{
				Limit: c.MemoryPerCoreLimit,
			},
		}
	}

	return c, warnings
}
