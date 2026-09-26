// Package version holds build information, set at link time:
//
//	go build -ldflags "-X github.com/wimmme/shellylanman/internal/version.Version=1.2.3 \
//	                   -X github.com/wimmme/shellylanman/internal/version.Commit=abc1234"
package version

var (
	// Version is the release version, or "dev" for local builds.
	Version = "dev"
	// Commit is the git commit the binary was built from, if known.
	Commit = ""
)
