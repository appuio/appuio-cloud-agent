package whoami_test

import (
	"context"
	"testing"

	"github.com/appuio/appuio-cloud-agent/whoami"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func Test_SelfSkipper_Skip(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, authenticationv1.AddToScheme(scheme))

	cfg, stop := setupEnvtestEnv(t)
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := testr.New(t)

	mgr, err := manager.New(cfg, manager.Options{
		Scheme: scheme,
		Logger: l,
	})
	require.NoError(t, err)

	subject, err := whoami.WhoamiForConfigAndClient(mgr.GetConfig(), mgr.GetHTTPClient())
	require.NoError(t, err)

	ssr, err := subject.Client.Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	t.Log(ssr)
	require.NoError(t, err)

	ui, err := subject.Whoami(ctx)
	assert.Equal(t, ssr.Status.UserInfo, ui)
}

func setupEnvtestEnv(t *testing.T) (cfg *rest.Config, stop func()) {
	t.Helper()

	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	require.NoError(t, err)

	return cfg, func() {
		require.NoError(t, testEnv.Stop())
	}
}
