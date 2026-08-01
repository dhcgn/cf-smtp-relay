package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print version, commit, and build time",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Direct CLI stdout output, not application logging.
			fmt.Printf("cf-smtp-relay %s (commit %s, built %s)\n", version, commit, buildTime)
			return nil
		},
	}
}
