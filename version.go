package main

import (
	"fmt"
	"runtime"
	"strings"
)

// Build-time variables that can be set with -ldflags
var (
	// Version is the application version, set at build time
	Version = "dev"
	
	// GitCommit is the git commit hash, set at build time
	GitCommit = "unknown"
	
	// GitBranch is the git branch, set at build time
	GitBranch = "unknown"
	
	// BuildTime is when the binary was built, set at build time
	BuildTime = "unknown"
	
	// BuildBy is who/what built the binary, set at build time
	BuildBy = "unknown"
)

// VersionInfo contains all version-related information
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	GitBranch string `json:"git_branch"`
	BuildTime string `json:"build_time"`
	BuildBy   string `json:"build_by"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// GetVersionInfo returns structured version information
func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   GetVersion(),
		GitCommit: GitCommit,
		GitBranch: GitBranch,
		BuildTime: BuildTime,
		BuildBy:   BuildBy,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// GetVersion returns the application version, formatted appropriately
func GetVersion() string {
	// If version is "dev", try to use git commit as version
	if Version == "dev" && GitCommit != "unknown" {
		// Truncate commit hash to 8 characters
		shortCommit := GitCommit
		if len(shortCommit) > 8 {
			shortCommit = shortCommit[:8]
		}
		
		// Add branch if it's not main/master
		if GitBranch != "unknown" && GitBranch != "main" && GitBranch != "master" {
			return fmt.Sprintf("dev-%s-%s", GitBranch, shortCommit)
		}
		
		return fmt.Sprintf("dev-%s", shortCommit)
	}
	
	// Return the version as-is if it's a proper release version
	return Version
}

// GetVersionString returns a human-readable version string
func GetVersionString() string {
	version := GetVersion()
	
	if Version == "dev" {
		return fmt.Sprintf("%s (commit: %s, branch: %s)", version, GitCommit, GitBranch)
	}
	
	return version
}

// IsDevelopmentBuild returns true if this is a development build
func IsDevelopmentBuild() bool {
	return Version == "dev" || strings.Contains(Version, "dev")
}