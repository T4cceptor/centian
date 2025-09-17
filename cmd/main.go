// Copyright 2025 CentianCLI Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"log"
	"os"

	"github.com/CentianAI/centian-cli/internal/cli"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/proxy"
	urfavecli "github.com/urfave/cli/v3"
)

// version is set by build flags during release.
var version = "dev"

func main() {
	// Create CLI app
	app := &urfavecli.Command{
		Name:                  "centian",
		Description:           "Proxy and modify your MCP server and tools.",
		Usage:                 "centian start",
		Version:               version,
		EnableShellCompletion: true,
		Commands: []*urfavecli.Command{
			cli.InitCommand,
			{
				Name:        "start",
				Usage:       "Start the MCP proxy",
				Description: "Starts the Centian MCP proxy server",
				Action: func(ctx context.Context, cmd *urfavecli.Command) error {
					return proxy.StartCentianProxy()
				},
			},
			config.ConfigCommand,
		},
	}

	// Run the CLI app
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
