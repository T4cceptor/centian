package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/urfave/cli/v3"
)

// ReloadCommand sends a reload request to a running Centian proxy server.
var ReloadCommand = &cli.Command{
	Name:  "reload",
	Usage: "Reload gateway configuration on a running Centian proxy server",
	Description: `Sends POST /admin/reload to the running Centian proxy server.

This terminates all active MCP sessions and reloads the gateway configuration
from the gateway provider (e.g. ~/.centian/gateways.json).

Examples:
  centian reload
  centian reload --host 127.0.0.1 --port 8080 --api-key mykey
  CENTIAN_API_KEY=mykey centian reload
`,
	Action: handleReloadCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "host",
			Usage: "Proxy host (default: value from config or 127.0.0.1)",
		},
		&cli.StringFlag{
			Name:  "port",
			Usage: "Proxy port (default: value from config or 8080)",
		},
		&cli.StringFlag{
			Name:    "api-key",
			Usage:   "API key for authentication (or set CENTIAN_API_KEY env var)",
			Sources: cli.EnvVars("CENTIAN_API_KEY"),
		},
		&cli.StringFlag{
			Name:  "config-path",
			Usage: "Path to config file used to read default host/port (default: ~/.centian/config.json)",
		},
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt",
		},
	},
}

type reloadResponse struct {
	Status           string `json:"status"`
	GatewaysReloaded int    `json:"gatewaysReloaded,omitempty"`
	Message          string `json:"message"`
}

func handleReloadCommand(_ context.Context, cmd *cli.Command) error {
	// Confirm unless --yes is set.
	if !cmd.Bool("yes") {
		fmt.Print("This will terminate all active MCP sessions. Continue? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Resolve host and port.
	host := cmd.String("host")
	port := cmd.String("port")
	if host == "" || port == "" {
		if cfg, err := loadConfigForReload(cmd.String("config-path")); err == nil {
			if host == "" && cfg.Proxy != nil {
				h := cfg.Proxy.Host
				if h == "" {
					h = config.DefaultProxyHost
				}
				host = h
			}
			if port == "" && cfg.Proxy != nil {
				port = cfg.Proxy.Port
			}
		}
		if host == "" {
			host = "127.0.0.1"
		}
		if port == "" {
			port = "8080"
		}
	}

	url := fmt.Sprintf("http://%s:%s/admin/reload", host, port)

	apiKey := cmd.String("api-key")

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Centian server at %s: %w", url, err)
	}
	defer resp.Body.Close()

	var reloadResp reloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&reloadResp); err != nil {
		return fmt.Errorf("unexpected response from server (status %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reload failed: %s", reloadResp.Message)
	}

	fmt.Printf("Reload successful: %d gateway(s) active.\n", reloadResp.GatewaysReloaded)
	return nil
}

func loadConfigForReload(configPath string) (*config.ServerConfig, error) {
	if configPath == "" {
		var err error
		configPath, err = config.GetConfigPath()
		if err != nil {
			return nil, err
		}
	}
	return config.LoadConfigFromPath(configPath)
}
