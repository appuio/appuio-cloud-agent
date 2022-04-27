package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/appuio/appuio-cloud-agent/controllers"
	"github.com/appuio/appuio-cloud-agent/ratio"
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

	memoryCPURatio := flag.String("memory-per-core-limit", "4Gi", "The fair use limit of memory usage per CPU core")
	organizationLabel := flag.String("organization-label", "appuio.io/organization", "The label used to mark namespaces to belong to an organization")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	ctx := ctrl.SetupSignalHandler()

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

	limit, err := resource.ParseQuantity(*memoryCPURatio)
	if err != nil {
		setupLog.Error(err, "unable to parse memory-per-core-limit")
		os.Exit(1)
	}
	mgr.GetWebhookServer().Register("/validate-request-ratio", &webhook.Admission{
		Handler: &webhooks.RatioValidator{
			RatioLimit: &limit,
			Ratio: &ratio.Fetcher{
				Client: mgr.GetClient(),
			},
		},
	})

	if err = (&controllers.RatioReconciler{
		Client:     mgr.GetClient(),
		Recorder:   mgr.GetEventRecorderFor("resource-ratio-controller"),
		Scheme:     mgr.GetScheme(),
		RatioLimit: &limit,
		Ratio: &ratio.Fetcher{
			Client:            mgr.GetClient(),
			OrganizationLabel: *organizationLabel,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ratio")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to setup health endpoint")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to setup ready endpoint")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
