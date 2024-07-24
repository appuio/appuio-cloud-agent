package clustersource

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// FromKubeConfig creates a ClusterSource from a kubeconfig.
func FromKubeConfig(kubeconfig []byte, scheme *runtime.Scheme) (cluster.Cluster, error) {
	rc, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create rest config from kubeconfig: %w", err)
	}
	clst, err := cluster.New(rc, func(o *cluster.Options) { o.Scheme = scheme })
	if err != nil {
		return nil, fmt.Errorf("unable to setup cluster: %w", err)
	}

	return clst, nil
}

// FromURLAndBearerToken creates a ClusterSource from a url and token.
// If more complex configuration is needed, use FromKubeConfig.
func FromURLAndBearerToken(url, token string, scheme *runtime.Scheme) (cluster.Cluster, error) {
	rc := &rest.Config{
		Host:        url, // yes this is the correct field, host accepts a url
		BearerToken: token,
	}

	clst, err := cluster.New(rc, func(o *cluster.Options) { o.Scheme = scheme })
	if err != nil {
		return nil, fmt.Errorf("unable to setup cluster: %w", err)
	}

	return clst, nil
}
