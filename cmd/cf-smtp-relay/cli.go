package main

import (
	"github.com/urfave/cli/v3"
)

func buildRootCommand() *cli.Command {
	return &cli.Command{
		Name:    "cf-smtp-relay",
		Usage:   "SMTP-to-Cloudflare-Email-API bridge",
		Version: version,
		Commands: []*cli.Command{
			serveCommand(),
			versionCommand(),
		},
	}
}
