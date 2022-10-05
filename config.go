package main

import (
	"os"

	"sigs.k8s.io/yaml"
)

type Config struct {
	// OrganizationLabel is the label used to mark namespaces to belong to an organization
	OrganizationLabel string

	// MemoryPerCoreLimit is the fair use limit of memory usage per CPU core
	MemoryPerCoreLimit string

	// Privileged* is a list of the given type allowed to bypass restrictions.
	// Wildcards are supported (e.g. "system:serviceaccount:default:*" or "cluster-*-operator").
	PrivilegedGroups       []string
	PrivilegedUsers        []string
	PrivilegedRoles        []string
	PrivilegedClusterRoles []string

	// AllowedNodeSelectors is a map of allowed node selectors.
	// Both the key and the value are anchored regexes.
	AllowedNodeSelectors map[string]string
}

func ConfigFromFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	return c, yaml.Unmarshal(raw, &c, yaml.DisallowUnknownFields)
}
