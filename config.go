package main

import (
	"errors"
	"os"

	"github.com/appuio/appuio-cloud-agent/limits"
	"go.uber.org/multierr"
	"sigs.k8s.io/yaml"
)

type Config struct {
	// OrganizationLabel is the label used to mark namespaces to belong to an organization
	OrganizationLabel string

	// MemoryPerCoreLimits is the fair use limit of memory usage per CPU core
	// It is possible to select limits by node selector labels
	MemoryPerCoreLimits limits.Limits

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
}

func ConfigFromFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	return c, yaml.Unmarshal(raw, &c, yaml.DisallowUnknownFields)
}

func (c Config) Validate() error {
	var errs []error

	if c.OrganizationLabel == "" {
		errs = append(errs, errors.New("OrganizationLabel must not be empty"))
	}

	return multierr.Combine(errs...)
}
