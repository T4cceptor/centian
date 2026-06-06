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

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/urfave/cli/v3"
)

// AuthCommand provides authentication utilities.
var AuthCommand = &cli.Command{
	Name:  "auth",
	Usage: "Manage Centian authentication",
	Commands: []*cli.Command{
		AuthNewKeyCommand,
	},
}

// AuthNewKeyCommand generates and stores a new API key.
var AuthNewKeyCommand = &cli.Command{
	Name:  "new-key",
	Usage: "centian auth new-key",
	Description: `Generate a new API key for the HTTP proxy.

The key is printed once to the console, then hashed with bcrypt and stored in the
configured auth backend. By default this is the principals SQLite database:
  ~/.centian/principals.sqlite

Use --type to choose the backend (sqlite or file) and --store to override its
location. When omitted, these fall back to the authBackend block in config.json,
then to the sqlite default.

Use --projects to restrict the key to specific projects (comma-separated slugs).
Omit the flag to allow all projects.

Use --name to label the key's principal. If omitted, you are prompted for one.
`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "projects",
			Usage: "Comma-separated list of project slugs this key is allowed to access (empty = all)",
		},
		&cli.StringFlag{
			Name:  "name",
			Usage: "Human-friendly name for this key's principal (prompted if omitted)",
		},
		&cli.StringFlag{
			Name:  "type",
			Usage: "Auth backend type: sqlite (default) or file",
		},
		&cli.StringFlag{
			Name:  "store",
			Usage: "Backend location (sqlite db path or key file path); defaults per backend type",
		},
	},
	Action: handleAuthNewKeyCommand,
}

// handleAuthNewKeyCommand generates and stores a new API key.
func handleAuthNewKeyCommand(ctx context.Context, cmd *cli.Command) error {
	name := strings.TrimSpace(cmd.String("name"))
	if name == "" {
		var err error
		name, err = promptLine(os.Stdin, os.Stdout, "Enter a name for this key (optional): ")
		if err != nil {
			return err
		}
	}

	backendType, store := resolveNewKeyBackend(cmd)

	var projects []string
	if p := cmd.String("projects"); p != "" {
		projects = parseCommaSeparated(p)
	}

	created, err := auth.CreateAPIKey(ctx, backendType, store, auth.CreateAPIKeyParams{
		Name:     name,
		Projects: projects,
	})
	if err != nil {
		return err
	}

	var pErr error
	if _, pErr = fmt.Fprintln(os.Stdout, "New API key (store this now, it won't be shown again):"); pErr != nil {
		return pErr
	}
	if _, pErr = fmt.Fprintln(os.Stdout, created.Token); pErr != nil {
		return pErr
	}
	if name != "" {
		if _, pErr = fmt.Fprintf(os.Stdout, "Name: %s\n", name); pErr != nil {
			return pErr
		}
	}
	if len(projects) > 0 {
		if _, pErr = fmt.Fprintf(os.Stdout, "Projects: %s\n", strings.Join(projects, ", ")); pErr != nil {
			return pErr
		}
	}
	if _, pErr = fmt.Fprintf(os.Stdout, "Stored hashed key in %s (%s backend)\n", created.Store, created.BackendType); pErr != nil {
		return pErr
	}
	return nil
}

// resolveNewKeyBackend determines the auth backend (type, store) for new-key.
// Explicit flags win; otherwise it falls back to the authBackend block in the
// loaded config, then to empty values (which the auth package resolves to the
// sqlite default). Config load failures (e.g. before `centian init`) are ignored
// so a key can still be minted with defaults.
func resolveNewKeyBackend(cmd *cli.Command) (backendType, store string) {
	backendType = strings.TrimSpace(cmd.String("type"))
	store = strings.TrimSpace(cmd.String("store"))
	if backendType != "" && store != "" {
		return backendType, store
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return backendType, store
	}
	cfgType, cfgStore := cfg.GetAuthBackend()
	if backendType == "" {
		backendType = cfgType
	}
	if store == "" {
		store = cfgStore
	}
	return backendType, store
}

// promptLine writes a label to out and reads a single trimmed line from in.
// An EOF (e.g. non-interactive/piped input with no data) yields an empty string
// rather than an error, so the name simply remains unset.
func promptLine(in io.Reader, out io.Writer, label string) (string, error) {
	if _, err := fmt.Fprint(out, label); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// parseCommaSeparated splits a comma-separated string into trimmed, non-empty parts.
func parseCommaSeparated(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
