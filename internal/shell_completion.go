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

package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShellInfo contains information about the current shell and its configuration
type ShellInfo struct {
	Name       string // bash, zsh, fish, etc.
	RCFile     string // path to RC file (~/.bashrc, ~/.zshrc, etc.)
	CompletionLine string // the line to add for completion
}

// DetectShell detects the current shell and returns shell information
func DetectShell() (*ShellInfo, error) {
	// Get shell from SHELL environment variable
	shell := os.Getenv("SHELL")
	if shell == "" {
		return nil, fmt.Errorf("unable to detect shell: SHELL environment variable not set")
	}

	// Extract shell name from path
	shellName := filepath.Base(shell)
	
	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("unable to get home directory: %w", err)
	}

	var info ShellInfo
	info.Name = shellName

	switch shellName {
	case "bash":
		// Try .bash_profile first (macOS), then .bashrc (Linux)
		bashProfile := filepath.Join(homeDir, ".bash_profile")
		bashrc := filepath.Join(homeDir, ".bashrc")
		
		if _, err := os.Stat(bashProfile); err == nil {
			info.RCFile = bashProfile
		} else {
			info.RCFile = bashrc
		}
		info.CompletionLine = "source <(centian completion bash)"
		
	case "zsh":
		info.RCFile = filepath.Join(homeDir, ".zshrc")
		info.CompletionLine = "source <(centian completion zsh)"
		
	case "fish":
		// Fish uses a different approach - completion files
		fishCompDir := filepath.Join(homeDir, ".config", "fish", "completions")
		info.RCFile = filepath.Join(fishCompDir, "centian.fish")
		info.CompletionLine = "" // Fish doesn't need a line in RC file
		
	default:
		return nil, fmt.Errorf("unsupported shell: %s", shellName)
	}

	return &info, nil
}

// SetupShellCompletion offers to set up shell completion for the user
func SetupShellCompletion() error {
	fmt.Println("\n🔧 Shell Completion Setup")
	fmt.Println("========================")

	// Detect shell
	shellInfo, err := DetectShell()
	if err != nil {
		fmt.Printf("⚠️  Could not detect shell: %s\n", err)
		fmt.Println("You can manually set up completion using: centian completion <shell>")
		return nil // Don't fail init if completion setup fails
	}

	fmt.Printf("📍 Detected shell: %s\n", shellInfo.Name)
	fmt.Printf("📁 Configuration file: %s\n", shellInfo.RCFile)
	
	if shellInfo.Name == "fish" {
		fmt.Println("\n💡 Fish shell uses a different completion system.")
		fmt.Printf("   Completion file will be created at: %s\n", shellInfo.RCFile)
		fmt.Println("   This will enable tab completion for centian commands.")
	} else {
		fmt.Println("\n💡 This will add the following line to your shell configuration:")
		fmt.Printf("   %s\n", shellInfo.CompletionLine)
		fmt.Println("   This enables tab completion for centian commands and subcommands.")
	}

	// Ask for user consent
	fmt.Print("\n❓ Would you like to set up shell completion? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("⏭️  Shell completion setup skipped.")
		fmt.Printf("   To set up later, run: centian completion %s\n", shellInfo.Name)
		return nil
	}

	// Set up completion based on shell type
	if shellInfo.Name == "fish" {
		return setupFishCompletion(shellInfo.RCFile)
	} else {
		return setupShellCompletion(shellInfo)
	}
}

// setupShellCompletion sets up completion for bash/zsh shells
func setupShellCompletion(shellInfo *ShellInfo) error {
	// Check if completion line already exists
	exists, err := completionExists(shellInfo.RCFile, shellInfo.CompletionLine)
	if err != nil {
		return fmt.Errorf("failed to check existing completion: %w", err)
	}

	if exists {
		fmt.Println("✅ Shell completion is already configured!")
		return nil
	}

	// Create RC file if it doesn't exist
	if _, err := os.Stat(shellInfo.RCFile); os.IsNotExist(err) {
		fmt.Printf("📄 Creating %s...\n", shellInfo.RCFile)
		file, err := os.Create(shellInfo.RCFile)
		if err != nil {
			return fmt.Errorf("failed to create RC file: %w", err)
		}
		file.Close()
	}

	// Add completion line
	file, err := os.OpenFile(shellInfo.RCFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open RC file: %w", err)
	}
	defer file.Close()

	completionBlock := fmt.Sprintf("\n# Centian CLI completion\n%s\n", shellInfo.CompletionLine)
	if _, err := file.WriteString(completionBlock); err != nil {
		return fmt.Errorf("failed to write completion line: %w", err)
	}

	fmt.Println("✅ Shell completion configured successfully!")
	fmt.Println("   Restart your shell or run 'source " + shellInfo.RCFile + "' to activate completion.")
	
	return nil
}

// setupFishCompletion sets up completion for fish shell
func setupFishCompletion(completionFile string) error {
	// Check if completion file already exists
	if _, err := os.Stat(completionFile); err == nil {
		fmt.Println("✅ Fish completion is already configured!")
		return nil
	}

	// Create completions directory if it doesn't exist
	completionDir := filepath.Dir(completionFile)
	if err := os.MkdirAll(completionDir, 0755); err != nil {
		return fmt.Errorf("failed to create completions directory: %w", err)
	}

	// Generate fish completion script
	fmt.Println("🐟 Generating fish completion script...")
	
	// We'll use the centian binary to generate the completion
	// For now, we'll create a simple script that calls the completion command
	fishScript := `# Centian CLI fish completion
complete -c centian -f -a "(centian --generate-shell-completion)"
`
	
	if err := os.WriteFile(completionFile, []byte(fishScript), 0644); err != nil {
		return fmt.Errorf("failed to write fish completion file: %w", err)
	}

	fmt.Println("✅ Fish completion configured successfully!")
	fmt.Println("   Fish will automatically load the completion on next shell start.")
	
	return nil
}

// completionExists checks if the completion line already exists in the RC file
func completionExists(rcFile, completionLine string) (bool, error) {
	file, err := os.Open(rcFile)
	if os.IsNotExist(err) {
		return false, nil // File doesn't exist, so completion doesn't exist
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == completionLine {
			return true, nil
		}
	}

	return false, scanner.Err()
}