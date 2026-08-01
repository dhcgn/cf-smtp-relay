// Command cf-smtp-relay is a tiny SMTP-to-Cloudflare-Email-API bridge.
package main

import (
	"context"
	"log/slog"
	"os"
)

// version, commit, and buildTime are set via -ldflags -X in CI; they default
// to these values for local, non-release builds.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if err := buildRootCommand().Run(context.Background(), os.Args); err != nil {
		slog.Error("cf-smtp-relay exited with error", "error", err)
		os.Exit(1)
	}
}
