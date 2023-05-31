package clustersource

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ClusterSource is a cluster with added methods to be used as a source for controller Watches.
type ClusterSource interface {
	cluster.Cluster

	// SourceFor returns a controller.Watches source for the given object.
	SourceFor(client.Object) source.SyncingSource
}

type clusterSource struct {
	cluster.Cluster
}

// FromKubeConfig creates a ClusterSource from a kubeconfig.
func FromKubeConfig(kubeconfig []byte, scheme *runtime.Scheme) (ClusterSource, error) {
	rc, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create rest config from kubeconfig: %w", err)
	}
	clst, err := cluster.New(rc, func(o *cluster.Options) { o.Scheme = scheme })
	if err != nil {
		return nil, fmt.Errorf("unable to setup cluster: %w", err)
	}

	return &clusterSource{
		Cluster: clst,
	}, nil
}

// FromURLAndBearerToken creates a ClusterSource from a url and token.
// If more complex configuration is needed, use FromKubeConfig.
func FromURLAndBearerToken(url, token string, scheme *runtime.Scheme) (ClusterSource, error) {
	rc := &rest.Config{
		Host:        url, // yes this is the correct field, host accepts a url
		BearerToken: token,
	}

	clst, err := cluster.New(rc, func(o *cluster.Options) { o.Scheme = scheme })
	if err != nil {
		return nil, fmt.Errorf("unable to setup cluster: %w", err)
	}

	return &clusterSource{
		Cluster: clst,
	}, nil
}

func (cs *clusterSource) SourceFor(obj client.Object) source.SyncingSource {
	return source.Kind(cs.GetCache(), obj)
}
