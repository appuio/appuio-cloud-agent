package main

import (
	"context"
	"flag"
	"os"
	"time"

	controlv1 "github.com/appuio/control-api/apis/v1"
	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	agentv1 "github.com/appuio/appuio-cloud-agent/api/v1"
	"github.com/appuio/appuio-cloud-agent/controllers"
	"github.com/appuio/appuio-cloud-agent/controllers/clustersource"
	"github.com/appuio/appuio-cloud-agent/controllers/transformers"
	"github.com/appuio/appuio-cloud-agent/ratio"
	"github.com/appuio/appuio-cloud-agent/skipper"
	"github.com/appuio/appuio-cloud-agent/webhooks"
)

var (
	// these variables are populated by Goreleaser when releasing
	version = "unknown"
	commit  = "-dirty-"
	date    = time.Now().Format("2006-01-02")

	appName     = "appuio-cloud-agent"
	appLongName = "agent running on every APPUiO Cloud Zone"

	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup").WithValues("version", version, "commit", commit, "date", date)
)

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen object paths="./..."
//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen rbac:roleName=appuio-cloud-agent webhook paths="./..."

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(userv1.AddToScheme(scheme))
	utilruntime.Must(projectv1.AddToScheme(scheme))
	utilruntime.Must(agentv1.AddToScheme(scheme))
	utilruntime.Must(controlv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	metricsAddr := flag.String("metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	enableLeaderElection := flag.Bool("leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	probeAddr := flag.String("health-probe-bind-address", ":8082", "The address the probe endpoint binds to.")
	webhookCertDir := flag.String("webhook-cert-dir", "", "Directory holding TLS certificate and key for the webhook server. If left empty, {TempDir}/k8s-webhook-server/serving-certs is used")
	webhookPort := flag.Int("webhook-port", 9443, "The port on which the admission webhooks are served")

	configFilePath := flag.String("config-file", "./config.yaml", "Path to the configuration file")

	var controlAPIKubeconfig string
	flag.StringVar(&controlAPIKubeconfig, "kubeconfig-control-api", "kubeconfig-control-api", "Path to the kubeconfig file to query the control API")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	conf, warnings, err := ConfigFromFile(*configFilePath)
	if err != nil {
		setupLog.Error(err, "unable to read config file")
		os.Exit(1)
	}
	for _, warning := range warnings {
		setupLog.Info("WARNING " + warning)
	}
	if err := conf.Validate(); err != nil {
		setupLog.Error(err, "invalid configuration")
		os.Exit(1)
	}

	if controlAPIKubeconfig == "" {
		setupLog.Info("no control API kubeconfig provided, aborting")
		os.Exit(1)
	}

	cac, err := os.ReadFile(controlAPIKubeconfig)
	if err != nil {
		setupLog.Error(err, "unable to read control API kubeconfig")
		os.Exit(1)
	}
	controlAPICluster, err := clustersource.FromKubeConfig(cac, scheme)
	if err != nil {
		setupLog.Error(err, "unable to setup control-api manager")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     *metricsAddr,
		Port:                   *webhookPort,
		HealthProbeBindAddress: *probeAddr,
		LeaderElection:         *enableLeaderElection,
		LeaderElectionID:       "f2g2bc31.appuio-cloud-agent.appuio.io",
		CertDir:                *webhookCertDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to setup manager")
		os.Exit(1)
	}

	registerRatioController(mgr, conf, conf.OrganizationLabel)
	registerOrganizationRBACController(mgr, conf.OrganizationLabel, conf.DefaultOrganizationClusterRoles)

	if err := (&controllers.ZoneUsageProfileSyncReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("usage-profile-sync-controller"),

		ForeignClient: controlAPICluster.GetClient(),
	}).SetupWithManagerAndForeignCluster(mgr, controlAPICluster); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ratio")
		os.Exit(1)
	}
	if err := (&controllers.ZoneUsageProfileApplyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("usage-profile-apply-controller"),

		OrganizationLabel: conf.OrganizationLabel,
		Transformers: []transformers.Transformer{
			transformers.NewResourceQuotaTransformer("resourcequota.appuio.io"),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ratio")
		os.Exit(1)
	}

	// Currently unused, but will be used for the next kyverno replacements
	psk := &skipper.PrivilegedUserSkipper{
		Client: mgr.GetClient(),

		PrivilegedUsers:        conf.PrivilegedUsers,
		PrivilegedGroups:       conf.PrivilegedGroups,
		PrivilegedClusterRoles: conf.PrivilegedClusterRoles,
	}

	registerNodeSelectorValidationWebhooks(mgr, conf)

	mgr.GetWebhookServer().Register("/validate-namespace-quota", &webhook.Admission{
		Handler: &webhooks.NamespaceQuotaValidator{
			Skipper: psk,
			Client:  mgr.GetClient(),

			OrganizationLabel:                 conf.OrganizationLabel,
			UserDefaultOrganizationAnnotation: conf.UserDefaultOrganizationAnnotation,

			DefaultNamespaceCountLimit: conf.DefaultNamespaceCountLimit,
		},
	})

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to setup health endpoint")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to setup ready endpoint")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
	shutdown := make(chan error)
	go func() {
		defer cancel()
		setupLog.Info("starting control-api manager")
		shutdown <- controlAPICluster.Start(ctx)
	}()
	go func() {
		defer cancel()
		setupLog.Info("starting manager")
		shutdown <- mgr.Start(ctx)
	}()
	if err := multierr.Combine(<-shutdown, <-shutdown); err != nil {
		setupLog.Error(err, "failed to start")
		os.Exit(1)
	}
}

func registerNodeSelectorValidationWebhooks(mgr ctrl.Manager, conf Config) {
	mgr.GetWebhookServer().Register("/mutate-pod-node-selector", &webhook.Admission{
		Handler: &webhooks.PodNodeSelectorMutator{
			Skipper:                                skipper.StaticSkipper{ShouldSkip: false},
			Client:                                 mgr.GetClient(),
			DefaultNodeSelector:                    conf.DefaultNodeSelector,
			DefaultNamespaceNodeSelectorAnnotation: conf.DefaultNamespaceNodeSelectorAnnotation,
		},
	})
}

func registerOrganizationRBACController(mgr ctrl.Manager, orgLabel string, defaultClusterRoles map[string]string) {
	if err := (&controllers.OrganizationRBACReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("organization-rbac-controller"),
		Scheme:   mgr.GetScheme(),

		OrganizationLabel:   orgLabel,
		DefaultClusterRoles: defaultClusterRoles,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ratio")
		os.Exit(1)
	}
}

func registerRatioController(mgr ctrl.Manager, conf Config, orgLabel string) {
	mgr.GetWebhookServer().Register("/validate-request-ratio", &webhook.Admission{
		Handler: &webhooks.RatioValidator{
			DefaultNodeSelector:                    conf.DefaultNodeSelector,
			DefaultNamespaceNodeSelectorAnnotation: conf.DefaultNamespaceNodeSelectorAnnotation,

			Client:      mgr.GetClient(),
			RatioLimits: conf.MemoryPerCoreLimits,
			Ratio: &ratio.Fetcher{
				Client: mgr.GetClient(),
			},
		},
	})

	if err := (&controllers.RatioReconciler{
		Client:      mgr.GetClient(),
		Recorder:    mgr.GetEventRecorderFor("resource-ratio-controller"),
		Scheme:      mgr.GetScheme(),
		RatioLimits: conf.MemoryPerCoreLimits,
		Ratio: &ratio.Fetcher{
			Client:            mgr.GetClient(),
			OrganizationLabel: orgLabel,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ratio")
		os.Exit(1)
	}
}
