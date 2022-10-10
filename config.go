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
	// ClusterRoles are only ever matched if they are bound through a ClusterRoleBinding,
	// this is different from the behavior of Kyverno.
	// This is done to prevent a user from wrongly configuring a low-privileged ClusterRole which users
	// can then bind to themselves to bypass the restrictions.
	PrivilegedGroups       []string
	PrivilegedUsers        []string
	PrivilegedClusterRoles []string

	// AllowedNodeSelectors is a map of allowed node selectors.
	// Both the key and the value are anchored regexes.
	AllowedNodeSelectors map[string]string

	// NamespaceDenyEmptyNodeSelector is a flag to enable or disable the rejection of empty node selectors on namespaces.
	// If true this will reject a { "openshift.io/node-selector": "" } annotation.
	NamespaceDenyEmptyNodeSelector bool

	//DefaultNodeSelectors are the default node selectors to add to pods if not set from namespace annotation
	DefaultNodeSelectors map[string]string
}

func ConfigFromFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	return c, yaml.Unmarshal(raw, &c, yaml.DisallowUnknownFields)
}
