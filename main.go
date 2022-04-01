package main

import (
	"time"
)

var (
	// these variables are populated by Goreleaser when releasing
	version = "unknown"
	commit  = "-dirty-"
	date    = time.Now().Format("2006-01-02")

	// TODO: Adjust app name
	appName     = "appuio-cloud-agent"
	appLongName = "agent running on every APPUiO Cloud Zone"

	// TODO: Adjust or clear env var prefix
	// envPrefix is the global prefix to use for the keys in environment variables
	envPrefix = "APPUIO_CLOUD_AGENT"
)

func main() {
}
