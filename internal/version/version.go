package version

import "runtime"

// These values are overridden at build time with -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func GoVersion() string {
	return runtime.Version()
}
