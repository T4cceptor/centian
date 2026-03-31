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
// - centian start.
// - centian auth.
// - centian server.
// - centian stdio.
// - centian logs.
// - centian config.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/T4cceptor/centian/internal/cli"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

// version is set by build flags during release.
var version = "dev"

func main() {
	if err := common.InitInternalLogger(common.LoggerOptions{}); err != nil {
		log.Printf("warning: failed to initialize internal logger: %v", err)
	}

	// Create CLI app.
	app := &urfavecli.Command{
		Name:                  "centian",
		Description:           "Proxy and modify MCP servers and tools.",
		Usage:                 "centian start",
		Version:               version,
		EnableShellCompletion: true,
		Commands: []*urfavecli.Command{
			cli.InitCommand,
			cli.StartCommand,
			cli.DemoCommand,
			cli.AuthCommand,
			config.ServerCommand,
			cli.ProcessorCommand,
			cli.LogsCommand,
			config.ConfigCommand,
		},
	}

	// Run the CLI app.
	exitCode := 0
	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
	if err := common.CloseLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to close internal logger: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
