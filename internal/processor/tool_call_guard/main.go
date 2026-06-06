package toolcallguard

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor/builtinutil"
)

// ProcessJSON processes one serialized Centian processor DataContext.
func ProcessJSON(input []byte, settings *config.BuiltinProcessorSettings) ([]byte, error) {
	ctx, err := builtinutil.DecodeContext(input)
	if err != nil {
		return nil, fmt.Errorf("decode processor input: %w", err)
	}

	_, err = builtinutil.ApplyToolGuard(ctx, builtinutil.ToolGuardOptions{
		Processor: config.BuiltinToolCallGuard,
		Mode:      settings.Mode,
		Rules:     guardRules(settings),
	})
	if err != nil {
		return nil, err
	}

	output, err := builtinutil.EncodeContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("encode processor output: %w", err)
	}
	return output, nil
}

func guardRules(settings *config.BuiltinProcessorSettings) []builtinutil.ToolGuardRule {
	rules := []builtinutil.ToolGuardRule{}
	for _, preset := range settings.Presets {
		switch preset {
		case config.BuiltinToolGuardPresetDangerousCommands:
			rules = append(rules, dangerousCommandRules()...)
		}
	}
	for _, rule := range settings.GuardRules {
		rules = append(rules, configRuleToBuiltinRule(rule))
	}
	return rules
}

func configRuleToBuiltinRule(rule config.BuiltinToolGuardRule) builtinutil.ToolGuardRule {
	argumentRules := make([]builtinutil.ToolGuardArgumentRule, 0, len(rule.ArgumentRules))
	for _, argumentRule := range rule.ArgumentRules {
		argumentRules = append(argumentRules, builtinutil.ToolGuardArgumentRule{
			Path:    argumentRule.Path,
			Pattern: argumentRule.Pattern,
		})
	}
	return builtinutil.ToolGuardRule{
		Name:          rule.Name,
		Severity:      rule.Severity,
		Message:       rule.Message,
		ToolPatterns:  rule.ToolPatterns,
		ArgumentRules: argumentRules,
	}
}

func dangerousCommandRules() []builtinutil.ToolGuardRule {
	toolPatterns := []string{
		"*shell*",
		"*exec*",
		"*command*",
		"*terminal*",
		"bash",
		"sh",
		"zsh",
		"powershell",
		"run_command",
		"run-command",
		"execute_command",
		"desktop_commander___*",
	}
	return []builtinutil.ToolGuardRule{
		dangerousCommandRule("dangerous_rm_rf", "Recursive force deletion commands are blocked.", toolPatterns, `(?i)(^|[\s;&|])rm\s+-(?:[^\s]*r[^\s]*f|[^\s]*f[^\s]*r)\b`),
		dangerousCommandRule("dangerous_remote_shell_pipe", "Piping downloaded scripts directly to a shell is blocked.", toolPatterns, `(?i)\b(curl|wget)\b[^\n|;&]*\|\s*(sh|bash|zsh|fish)\b`),
		dangerousCommandRule("dangerous_chmod_777", "World-writable chmod operations are blocked.", toolPatterns, `(?i)\bchmod\s+(?:-[^\s]+\s+)?777\b`),
		dangerousCommandRule("dangerous_sudo", "sudo commands are blocked.", toolPatterns, `(?i)(^|[\s;&|])sudo\b`),
		dangerousCommandRule("dangerous_disk_operation", "Disk formatting or destructive disk operations are blocked.", toolPatterns, `(?i)\b(mkfs(?:\.[a-z0-9]+)?|dd\s+if=|diskutil\s+eraseDisk|format\s+[A-Z]:|wipefs|shred)\b`),
		dangerousCommandRule("dangerous_git_force_push", "Force-pushing Git refs is blocked.", toolPatterns, `(?i)\bgit\s+push\b[^\n;]*\s--force(?:-with-lease)?\b`),
		dangerousCommandRule("dangerous_kubectl_delete", "kubectl delete operations are blocked.", toolPatterns, `(?i)\bkubectl\s+delete\b`),
		dangerousCommandRule("dangerous_terraform_destroy", "terraform destroy operations are blocked.", toolPatterns, `(?i)\bterraform\s+destroy\b`),
		dangerousCommandRule("dangerous_npm_publish", "npm publish operations are blocked.", toolPatterns, `(?i)\bnpm\s+publish\b`),
		dangerousCommandRule("dangerous_docker_system_prune", "docker system prune operations are blocked.", toolPatterns, `(?i)\bdocker\s+system\s+prune\b`),
	}
}

func dangerousCommandRule(name string, message string, toolPatterns []string, pattern string) builtinutil.ToolGuardRule {
	return builtinutil.ToolGuardRule{
		Name:         name,
		Severity:     "high",
		Message:      message,
		ToolPatterns: toolPatterns,
		ArgumentRules: []builtinutil.ToolGuardArgumentRule{
			{Path: "command", Pattern: pattern},
			{Path: "cmd", Pattern: pattern},
			{Path: "script", Pattern: pattern},
			{Path: "input", Pattern: pattern},
			{Path: "*.command", Pattern: pattern},
			{Path: "*.cmd", Pattern: pattern},
			{Path: "*.script", Pattern: pattern},
			{Path: "*.input", Pattern: pattern},
			{Path: "*.*.command", Pattern: pattern},
			{Path: "*.*.cmd", Pattern: pattern},
			{Path: "*.*.script", Pattern: pattern},
			{Path: "*.*.input", Pattern: pattern},
		},
	}
}
