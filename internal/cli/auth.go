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
auth backend defined by your Centian config (the global "authBackend" block).
By default this is the principals SQLite database:
  ~/.centian/principals.sqlite

Use --config to point at a specific config file; its authBackend decides where the
key is stored, keeping this command in sync with the server that reads it. When
omitted, the default config (~/.centian/config.json) is used, falling back to the
sqlite default if no config exists yet.

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
			Name:  "config",
			Usage: "Path to a Centian config whose authBackend selects where the key is stored (defaults to ~/.centian/config.json)",
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

	backendType, store, err := resolveNewKeyBackend(cmd)
	if err != nil {
		return err
	}

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

// resolveNewKeyBackend derives the auth backend (type, store) for new-key from a
// Centian config so the command always writes where the server reads.
//
// With --config, the named file's authBackend block is used and load failures are
// fatal (the operator explicitly pointed at it). Without --config, the default
// config is used when present; a missing/invalid default config is not fatal so a
// key can still be minted with backend defaults (sqlite) before `centian init`.
func resolveNewKeyBackend(cmd *cli.Command) (backendType, store string, err error) {
	if configPath := strings.TrimSpace(cmd.String("config")); configPath != "" {
		cfg, loadErr := config.LoadConfigFromPath(configPath)
		if loadErr != nil {
			return "", "", fmt.Errorf("failed to load config %q: %w", configPath, loadErr)
		}
		backendType, store = cfg.GetAuthBackend()
		return backendType, store, nil
	}

	// Best-effort: a missing/invalid default config falls back to backend defaults.
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		return "", "", nil
	}
	backendType, store = cfg.GetAuthBackend()
	return backendType, store, nil
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
