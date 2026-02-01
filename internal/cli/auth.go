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
	"context"
	"fmt"
	"os"

	"github.com/T4cceptor/centian/internal/auth"
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

The key is printed once to the console, then hashed with bcrypt and stored in:
  ~/.centian/api_keys.json
`,
	Action: handleAuthNewKeyCommand,
}

// handleAuthNewKeyCommand generates and stores a new API key.
func handleAuthNewKeyCommand(_ context.Context, _ *cli.Command) error {
	path, err := auth.DefaultAPIKeysPath()
	if err != nil {
		return fmt.Errorf("failed to resolve api key path: %w", err)
	}

	key, err := auth.GenerateAPIKey()
	if err != nil {
		return err
	}

	var pErr error
	_, pErr = fmt.Fprintln(os.Stdout, "New API key (store this now, it won't be shown again):")
	if pErr != nil {
		return pErr
	}
	_, pErr = fmt.Fprintln(os.Stdout, key)
	if pErr != nil {
		return pErr
	}

	entry, err := auth.NewAPIKeyEntry(key)
	if err != nil {
		return err
	}

	if _, err := auth.AppendAPIKey(path, entry); err != nil {
		return err
	}

	_, pErr = fmt.Fprintf(os.Stdout, "Stored hashed key in %s\n", path)
	if pErr != nil {
		return pErr
	}
	return nil
}
