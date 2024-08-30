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
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

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

	var controlAPIURL string
	flag.StringVar(&controlAPIURL, "control-api-url", "", "URL of the control API. If set agent does not use `-kubeconfig-control-api`. Expects a bearer token in `CONTROL_API_BEARER_TOKEN` env var.")

	var upstreamZoneIdentifier string
	flag.StringVar(&upstreamZoneIdentifier, "upstream-zone-identifier", "", "Identifies the agent in the control API. Currently used for Team/OrganizationMembers finalizer. Must be set if the GroupSync controller is enabled.")

	var selectedUsageProfile string
	flag.StringVar(&selectedUsageProfile, "usage-profile", "", "UsageProfile to use. Applies all profiles if empty. Dynamic selection is not supported yet.")

	var cloudscaleLoadbalancerValidationEnabled bool
	flag.BoolVar(&cloudscaleLoadbalancerValidationEnabled, "cloudscale-loadbalancer-validation-enabled", false, "Enable Cloudscale Loadbalancer validation. Validates that the k8s.cloudscale.ch/loadbalancer-uuid annotation cannot be changed by unprivileged users.")

	var namespaceProjectOrganizationMutatorEnabled bool
	flag.BoolVar(&namespaceProjectOrganizationMutatorEnabled, "namespace-project-organization-mutator-enabled", false, "Enable the NamespaceProjectOrganizationMutator webhook. Adds the organization label to namespace and project create requests.")

	var namespaceMetadataValidatorEnabled bool
	flag.BoolVar(&namespaceMetadataValidatorEnabled, "namespace-metadata-validator-enabled", false, "Enable the NamespaceMetadataValidator webhook. Validates the metadata of a namespace.")

	var legacyNamespaceQuotaEnabled bool
	flag.BoolVar(&legacyNamespaceQuotaEnabled, "legacy-namespace-quota-enabled", false, "Enable the legacy namespace quota controller. This controller is deprecated and will be removed in the future.")

	var qps, burst int
	flag.IntVar(&qps, "qps", 20, "QPS to use for the controller-runtime client")
	flag.IntVar(&burst, "burst", 100, "Burst to use for the controller-runtime client")

	var disableUserAttributeSync, disableGroupSync, disableUsageProfiles bool
	flag.BoolVar(&disableUserAttributeSync, "disable-user-attribute-sync", false, "Disable the UserAttributeSync controller")
	flag.BoolVar(&disableGroupSync, "disable-group-sync", false, "Disable the GroupSync controller")
	flag.BoolVar(&disableUsageProfiles, "disable-usage-profiles", false, "Disable the UsageProfile controllers")

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

	var controlAPICluster cluster.Cluster
	if controlAPIURL != "" {
		tk := os.Getenv("CONTROL_API_BEARER_TOKEN")
		if tk == "" {
			setupLog.Error(err, "CONTROL_API_BEARER_TOKEN env var not set")
			os.Exit(1)
		}
		cl, err := clustersource.FromURLAndBearerToken(controlAPIURL, tk, scheme)
		if err != nil {
			setupLog.Error(err, "unable to setup control-api manager")
			os.Exit(1)
		}
		controlAPICluster = cl
	} else {
		cac, err := os.ReadFile(controlAPIKubeconfig)
		if err != nil {
			setupLog.Error(err, "unable to read control API kubeconfig")
			os.Exit(1)
		}
		cl, err := clustersource.FromKubeConfig(cac, scheme)
		if err != nil {
			setupLog.Error(err, "unable to setup control-api manager")
			os.Exit(1)
		}
		controlAPICluster = cl
	}

	lconf := ctrl.GetConfigOrDie()
	lconf.QPS = float32(qps)
	lconf.Burst = burst
	mgr, err := ctrl.NewManager(lconf, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: *metricsAddr,
		},
		HealthProbeBindAddress: *probeAddr,
		LeaderElection:         *enableLeaderElection,
		LeaderElectionID:       "f2g2bc31.appuio-cloud-agent.appuio.io",
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    *webhookPort,
			CertDir: *webhookCertDir,
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to setup manager")
		os.Exit(1)
	}

	registerRatioController(mgr, conf, conf.OrganizationLabel)
	registerOrganizationRBACController(mgr, conf.OrganizationLabel, conf.DefaultOrganizationClusterRoles)

	if !disableUserAttributeSync {
		if err := (&controllers.UserAttributeSyncReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("user-attribute-sync-controller"),

			ForeignClient: controlAPICluster.GetClient(),
		}).SetupWithManagerAndForeignCluster(mgr, controlAPICluster); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "UserAttributeSync")
			os.Exit(1)
		}
	}
	if !disableGroupSync {
		if upstreamZoneIdentifier == "" {
			setupLog.Error(err, "upstream-zone-identifier must be set if GroupSync controller is enabled")
			os.Exit(1)
		}
		if err := (&controllers.GroupSyncReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("group-sync-controller"),

			ForeignClient: controlAPICluster.GetClient(),

			ControlAPIFinalizerZoneName: upstreamZoneIdentifier,
		}).SetupWithManagerAndForeignCluster(mgr, controlAPICluster); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "GroupSync")
			os.Exit(1)
		}
	}

	if !disableUsageProfiles {
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
			Cache:    mgr.GetCache(),

			OrganizationLabel: conf.OrganizationLabel,
			Transformers: []transformers.Transformer{
				transformers.NewResourceQuotaTransformer("resourcequota.appuio.io"),
			},

			SelectedProfile: selectedUsageProfile,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ratio")
			os.Exit(1)
		}
	}

	psk := &skipper.PrivilegedUserSkipper{
		Client: mgr.GetClient(),

		PrivilegedUsers:        conf.PrivilegedUsers,
		PrivilegedGroups:       conf.PrivilegedGroups,
		PrivilegedClusterRoles: conf.PrivilegedClusterRoles,
	}

	registerNodeSelectorValidationWebhooks(mgr, conf)

	mgr.GetWebhookServer().Register("/validate-namespace-quota", &webhook.Admission{
		Handler: &webhooks.NamespaceQuotaValidator{
			Client:  mgr.GetClient(),
			Decoder: admission.NewDecoder(mgr.GetScheme()),

			Skipper: psk,

			SkipValidateQuota: disableUsageProfiles && !legacyNamespaceQuotaEnabled,

			OrganizationLabel:                 conf.OrganizationLabel,
			UserDefaultOrganizationAnnotation: conf.UserDefaultOrganizationAnnotation,

			SelectedProfile:        selectedUsageProfile,
			QuotaOverrideNamespace: conf.QuotaOverrideNamespace,

			EnableLegacyNamespaceQuota: legacyNamespaceQuotaEnabled,
			LegacyNamespaceQuota:       conf.LegacyNamespaceQuota,
		},
	})

	mgr.GetWebhookServer().Register("/validate-service-cloudscale-lb", &webhook.Admission{
		Handler: &webhooks.ServiceCloudscaleLBValidator{
			Decoder: admission.NewDecoder(mgr.GetScheme()),
			Skipper: skipper.NewMultiSkipper(
				skipper.StaticSkipper{ShouldSkip: !cloudscaleLoadbalancerValidationEnabled},
				psk,
			),
		},
	})

	mgr.GetWebhookServer().Register("/mutate-namespace-project-organization", &webhook.Admission{
		Handler: &webhooks.NamespaceProjectOrganizationMutator{
			Decoder: admission.NewDecoder(mgr.GetScheme()),
			Client:  mgr.GetClient(),

			Skipper: skipper.NewMultiSkipper(
				skipper.StaticSkipper{ShouldSkip: !namespaceProjectOrganizationMutatorEnabled},
				psk,
			),

			OrganizationLabel:                 conf.OrganizationLabel,
			UserDefaultOrganizationAnnotation: conf.UserDefaultOrganizationAnnotation,
		},
	})

	mgr.GetWebhookServer().Register("/validate-namespace-metadata", &webhook.Admission{
		Handler: &webhooks.NamespaceMetadataValidator{
			Decoder: admission.NewDecoder(mgr.GetScheme()),
			Skipper: skipper.NewMultiSkipper(
				skipper.StaticSkipper{ShouldSkip: !namespaceMetadataValidatorEnabled},
				psk,
			),

			ReservedNamespaces: conf.ReservedNamespaces,
			AllowedAnnotations: conf.AllowedAnnotations,
			AllowedLabels:      conf.AllowedLabels,
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
			Client:  mgr.GetClient(),
			Decoder: admission.NewDecoder(mgr.GetScheme()),

			Skipper: skipper.StaticSkipper{ShouldSkip: false},

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

			Decoder: admission.NewDecoder(mgr.GetScheme()),
			Client:  mgr.GetClient(),

			RatioLimits: conf.MemoryPerCoreLimits,
			Ratio: &ratio.Fetcher{
				Client: mgr.GetClient(),
			},
			RatioWarnThreshold: conf.MemoryPerCoreWarnThreshold,
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
		RatioWarnThreshold: conf.MemoryPerCoreWarnThreshold,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ratio")
		os.Exit(1)
	}
}
