// Copyright 2026 Centian Contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at.
//
//     http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main contains the entry point for centian - it uses internal packages to provide the following CLI commands:.
// - centian init.
// - centian stdio.
// - centian server.
// - centian logs.
// - centian config.
package main

import (
	"context"
	"log"
	"os"

	"github.com/T4cceptor/centian/internal/cli"
	"github.com/T4cceptor/centian/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

// version is set by build flags during release.
var version = "dev"

func main() {
	// Create CLI app.
	app := &urfavecli.Command{
		Name:                  "centian",
		Description:           "Proxy and modify your MCP server and tools.",
		Usage:                 "centian start",
		Version:               version,
		EnableShellCompletion: true,
		Commands: []*urfavecli.Command{
			cli.InitCommand,
			cli.ServerCommand,
			cli.ProcessorCommand,
			cli.LogsCommand,
			config.ConfigCommand,
		},
	}

	// Run the CLI app.
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
