// Package version is the single source of truth for the dscli version string.
// It is updated at build time via ldflags.
package version

// Version is the current dscli version.
// Overridden at build time via: -X github.com/dscli/dscli/internal/version.Version=$(VERSION)
var Version = "0.8.6"
