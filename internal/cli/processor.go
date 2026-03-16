package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor"
	"github.com/urfave/cli/v3"
)

const renameOption = "rename"
const overwriteOption = "overwrite"
const abortOption = "abort"

// ProcessorCommand provides processor management functionality.
var ProcessorCommand = &cli.Command{
	Name:  "processor",
	Usage: "Manage Centian processors",
	Commands: []*cli.Command{
		ProcessorInitCommand,
		ProcessorAddCommand,
	},
}

// ProcessorInitCommand scaffolds a new processor.
var ProcessorInitCommand = &cli.Command{
	Name:        "new",
	Usage:       "centian processor new",
	Description: "Interactively scaffold a new processor.",
	Action:      handleProcessorInit,
}

// ProcessorAddCommand registers an existing processor script in the global config.
var ProcessorAddCommand = &cli.Command{
	Name:        "add",
	Usage:       "Add a processor to the configuration",
	Description: "Register a processor script in the global config. Name is inferred from the filename unless --name is provided.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Usage:   "Path to the processor script (required for --type cli)",
		},
		&cli.StringFlag{
			Name:    "name",
			Aliases: []string{"n"},
			Usage:   "Processor name (default: inferred from filename)",
		},
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			Usage:   "Processor type",
			Value:   "cli",
		},
		&cli.StringFlag{
			Name:  "url",
			Usage: "Webhook URL (required for --type webhook)",
		},
		&cli.StringSliceFlag{
			Name:  "header",
			Usage: "Webhook header in Key=Value form (repeatable, only for --type webhook)",
			Value: []string{},
		},
	},
	Action: handleProcessorAdd,
}

func handleProcessorInit(_ context.Context, _ *cli.Command) error {
	return processor.RunScaffoldInteractive(os.Stdin, os.Stdout)
}

func handleProcessorAdd(_ context.Context, cmd *cli.Command) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	path := cmd.String("path")
	processorType := cmd.String("type")
	urlValue := cmd.String("url")
	headerValues := cmd.StringSlice("header")

	// Infer name from filename if not provided.
	name := cmd.String("name")
	if name == "" {
		switch processorType {
		case string(config.CLIProcessor):
			name = config.InferProcessorNameFromPath(path)
		case string(config.WebhookProcessor):
			name = config.InferProcessorNameFromWebhookURL(urlValue)
		default:
			name = config.InferProcessorNameFromPath(path)
		}
	}

	// Handle duplicate name.
	if cfg.HasProcessor(name) {
		action, resolvedName, promptErr := promptProcessorNameConflict(name, os.Stdin)
		if promptErr != nil {
			return promptErr
		}
		switch action {
		case renameOption:
			name = resolvedName
		case overwriteOption:
			// Will replace below.
		case abortOption:
			fmt.Println("❌ Operation cancelled.")
			return nil
		}
	}

	enabled := true
	processorConfig := &config.ProcessorConfig{
		Name:    name,
		Type:    processorType,
		Enabled: enabled,
		Timeout: 15,
	}
	var summary string

	switch processorType {
	case string(config.CLIProcessor):
		if path == "" {
			return fmt.Errorf("--path is required for cli processors")
		}
		if urlValue != "" {
			return fmt.Errorf("--url is only supported for webhook processors")
		}
		if len(headerValues) > 0 {
			return fmt.Errorf("--header is only supported for webhook processors")
		}

		command, args, err := config.InferCommandFromPath(path)
		if err != nil {
			return err
		}

		configArgs := make([]interface{}, len(args))
		for i, a := range args {
			configArgs[i] = a
		}
		processorConfig.Config = map[string]interface{}{
			"command": command,
			"args":    configArgs,
		}
		summary = fmt.Sprintf("command: %s %s", command, strings.Join(args, " "))
	case string(config.WebhookProcessor):
		if path != "" {
			return fmt.Errorf("--path is not supported for webhook processors")
		}
		if urlValue == "" {
			return fmt.Errorf("--url is required for webhook processors")
		}

		headers, err := parseProcessorHeaderFlags(headerValues)
		if err != nil {
			return err
		}
		headerConfig := make(map[string]interface{}, len(headers))
		for key, value := range headers {
			headerConfig[key] = value
		}

		processorConfig.Config = map[string]interface{}{
			"url": urlValue,
		}
		if len(headerConfig) > 0 {
			processorConfig.Config["headers"] = headerConfig
		}
		summary = fmt.Sprintf("webhook: %s", urlValue)
	default:
		return fmt.Errorf("unsupported processor type '%s'", processorType)
	}

	// Add or replace.
	if cfg.HasProcessor(name) {
		cfg.ReplaceProcessor(name, processorConfig)
	} else {
		cfg.AddProcessor(processorConfig)
	}

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✅ Added processor '%s' (%s)\n", name, summary)
	return nil
}

func parseProcessorHeaderFlags(values []string) (map[string]string, error) {
	headers := make(map[string]string, len(values))
	for _, value := range values {
		key, headerValue, ok := strings.Cut(value, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --header value %q: expected Key=Value", value)
		}
		headers[key] = headerValue
	}
	return headers, nil
}

// promptProcessorNameConflict handles the interactive conflict resolution when a
// processor name already exists. Returns the chosen action and resolved name.
func promptProcessorNameConflict(name string, input io.Reader) (action, resolvedName string, err error) {
	suffix, err := generateRandomSuffix(4)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate random suffix: %w", err)
	}
	suggested := name + "_" + suffix

	fmt.Printf("\n⚠️  Processor '%s' already exists.\n\n", name)
	fmt.Printf("  [1] Rename to '%s'\n", suggested)
	fmt.Printf("  [2] Overwrite existing processor\n")
	fmt.Printf("  [3] Abort\n\n")
	fmt.Print("Select [1-3]: ")

	reader := bufio.NewReader(input)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("failed to read input: %w", err)
	}
	response = strings.TrimSpace(response)

	switch response {
	case "1":
		return renameOption, suggested, nil
	case "2":
		return overwriteOption, name, nil
	default:
		return abortOption, "", nil
	}
}

const randomSuffixChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// generateRandomSuffix generates a random alphanumeric string of the given length.
func generateRandomSuffix(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomSuffixChars))))
		if err != nil {
			return "", err
		}
		result[i] = randomSuffixChars[idx.Int64()]
	}
	return string(result), nil
}
