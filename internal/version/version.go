package version

import (
	"fmt"
	"runtime"
)

var (
	// These variables are replaced by ldflags at build time
	Version   = "v0.5.6"
	CommitSHA = "none"
	BuildTime = "unknown"
)

// Print outputs the version information to stdout
func Print() {
	fmt.Printf("Version: %s\nCommit: %s\nBuilt: %s\nGo: %s\nOS/Arch: %s/%s\n",
		Version, CommitSHA, BuildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// Info returns a formatted string with version details
func Info() string {
	return fmt.Sprintf("%s (commit: %s, built: %s)", Version, CommitSHA, BuildTime)
}
