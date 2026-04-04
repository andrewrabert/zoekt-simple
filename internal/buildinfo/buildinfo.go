// Package buildinfo exposes version information embedded at build time via
// -ldflags. When not set, the variables default to "dev".
package buildinfo

// These variables are set at build time via -ldflags:
//
//	-X github.com/sourcegraph/zoekt-simple/internal/buildinfo.Version=v0.1.0
//	-X github.com/sourcegraph/zoekt-simple/internal/buildinfo.UpstreamCommit=abc1234
var (
	// Version is the zoekt-simple version, derived from git tags/commits
	// (e.g. "v0.1.0" or "v0.1.0-3-gabcdef").
	Version = "dev"

	// UpstreamCommit is the commit SHA of the upstream zoekt fork built
	// into this binary.
	UpstreamCommit = "unknown"
)

// Info returns the version information as a structured type.
func Info() VersionInfo {
	return VersionInfo{
		Version:        Version,
		UpstreamCommit: UpstreamCommit,
	}
}

// VersionInfo holds structured version information for JSON serialization.
type VersionInfo struct {
	Version        string `json:"version"`
	UpstreamCommit string `json:"upstream_commit"`
}
