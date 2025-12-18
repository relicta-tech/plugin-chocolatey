// Package main implements the Chocolatey plugin for Relicta.
package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// Security validation patterns.
var (
	// Package path pattern: alphanumerics, dots, dashes, underscores, forward slashes.
	// Case-insensitive for the .nupkg extension.
	packagePathPattern = regexp.MustCompile(`(?i)^[a-zA-Z0-9][a-zA-Z0-9._/-]*\.nupkg$`)
)

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// RealCommandExecutor executes actual system commands.
type RealCommandExecutor struct{}

// Run executes a command and returns combined output.
func (e *RealCommandExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// ChocolateyPlugin implements the Publish packages to Chocolatey (Windows) plugin.
type ChocolateyPlugin struct {
	// cmdExecutor is used for executing shell commands. If nil, uses RealCommandExecutor.
	cmdExecutor CommandExecutor
}

// getExecutor returns the command executor, defaulting to RealCommandExecutor.
func (p *ChocolateyPlugin) getExecutor() CommandExecutor {
	if p.cmdExecutor != nil {
		return p.cmdExecutor
	}
	return &RealCommandExecutor{}
}

// Config represents the Chocolatey plugin configuration.
type Config struct {
	APIKey      string
	Source      string
	PackagePath string
	Timeout     int
	Force       bool
}

// GetInfo returns plugin metadata.
func (p *ChocolateyPlugin) GetInfo() plugin.Info {
	return plugin.Info{
		Name:        "chocolatey",
		Version:     "2.0.0",
		Description: "Publish packages to Chocolatey (Windows)",
		Author:      "Relicta Team",
		Hooks: []plugin.Hook{
			plugin.HookPostPublish,
		},
		ConfigSchema: `{
			"type": "object",
			"properties": {
				"api_key": {"type": "string", "description": "Chocolatey API key (or use CHOCOLATEY_API_KEY env)"},
				"source": {"type": "string", "description": "Chocolatey source URL", "default": "https://push.chocolatey.org/"},
				"package_path": {"type": "string", "description": "Path to .nupkg file (supports {{version}} placeholder)"},
				"timeout": {"type": "integer", "description": "Push timeout in seconds", "default": 300},
				"force": {"type": "boolean", "description": "Force push even if package exists", "default": false}
			},
			"required": ["package_path"]
		}`,
	}
}

// Execute runs the plugin for a given hook.
func (p *ChocolateyPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	cfg := p.parseConfig(req.Config)

	switch req.Hook {
	case plugin.HookPostPublish:
		return p.pushPackage(ctx, cfg, req.Context, req.DryRun)
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// pushPackage executes the choco push command.
func (p *ChocolateyPlugin) pushPackage(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	// Resolve version placeholder in package path.
	version := strings.TrimPrefix(releaseCtx.Version, "v")
	packagePath := strings.ReplaceAll(cfg.PackagePath, "{{version}}", version)
	packagePath = strings.ReplaceAll(packagePath, "{{tag}}", releaseCtx.TagName)

	// Validate package path.
	if err := validatePackagePath(packagePath); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid package path: %v", err),
		}, nil
	}

	// Validate source URL.
	if err := validateSourceURL(cfg.Source); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid source URL: %v", err),
		}, nil
	}

	// Validate API key is present.
	if cfg.APIKey == "" {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   "API key is required: set api_key in config or CHOCOLATEY_API_KEY environment variable",
		}, nil
	}

	if dryRun {
		return &plugin.ExecuteResponse{
			Success: true,
			Message: "Would push Chocolatey package",
			Outputs: map[string]any{
				"package_path": packagePath,
				"source":       cfg.Source,
				"version":      version,
				"force":        cfg.Force,
				"timeout":      cfg.Timeout,
			},
		}, nil
	}

	// Build command arguments.
	args := p.buildPushArgs(cfg, packagePath)

	// Create context with timeout.
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	// Execute choco push.
	output, err := p.getExecutor().Run(execCtx, "choco", args...)
	if err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("choco push failed: %v\nOutput: %s", err, string(output)),
		}, nil
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully pushed Chocolatey package %s to %s", packagePath, cfg.Source),
		Outputs: map[string]any{
			"package_path": packagePath,
			"source":       cfg.Source,
			"version":      version,
			"output":       string(output),
		},
	}, nil
}

// buildPushArgs constructs the command line arguments for choco push.
func (p *ChocolateyPlugin) buildPushArgs(cfg *Config, packagePath string) []string {
	args := []string{"push", packagePath}

	// API key.
	args = append(args, "--api-key", cfg.APIKey)

	// Source URL.
	args = append(args, "--source", cfg.Source)

	// Timeout.
	args = append(args, "--timeout", fmt.Sprintf("%d", cfg.Timeout))

	// Force flag.
	if cfg.Force {
		args = append(args, "--force")
	}

	return args
}

// validatePackagePath validates a package path to prevent path traversal and injection.
func validatePackagePath(path string) error {
	if path == "" {
		return fmt.Errorf("package path cannot be empty")
	}

	// Check length.
	if len(path) > 512 {
		return fmt.Errorf("package path too long (max 512 characters)")
	}

	// Clean the path.
	cleaned := filepath.Clean(path)

	// Check for path traversal attempts.
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+"..") {
		return fmt.Errorf("path traversal detected: cannot use '..' to escape working directory")
	}

	// Must end with .nupkg.
	if !strings.HasSuffix(strings.ToLower(path), ".nupkg") {
		return fmt.Errorf("package path must end with .nupkg")
	}

	// Basic pattern validation for the filename part.
	base := filepath.Base(path)
	if !packagePathPattern.MatchString(base) {
		return fmt.Errorf("invalid package filename: contains disallowed characters")
	}

	return nil
}

// validateSourceURL validates that a source URL is safe (SSRF protection).
func validateSourceURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("source URL cannot be empty")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := parsedURL.Hostname()

	// Allow localhost for testing purposes.
	isLocalhost := host == "localhost" || host == "127.0.0.1" || host == "::1"

	// Require HTTPS for non-localhost URLs.
	if parsedURL.Scheme != "https" && !isLocalhost {
		return fmt.Errorf("only HTTPS URLs are allowed (got %s)", parsedURL.Scheme)
	}

	// Allow HTTP for localhost.
	if isLocalhost && parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme for localhost (expected http or https)")
	}

	// For localhost, skip private IP check (it's intentionally local).
	if isLocalhost {
		return nil
	}

	// Resolve hostname to check for private IPs.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("URLs pointing to private networks are not allowed")
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	// Private IPv4 ranges.
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16", // Link-local
		"0.0.0.0/8",
	}

	// Cloud metadata endpoints.
	cloudMetadata := []string{
		"169.254.169.254/32", // AWS/GCP/Azure metadata
		"fd00:ec2::254/128",  // AWS IMDSv2 IPv6
	}

	allRanges := append(privateRanges, cloudMetadata...)

	for _, cidr := range allRanges {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if block.Contains(ip) {
			return true
		}
	}

	// Check for IPv6 private ranges.
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}

	return false
}

// Validate validates the plugin configuration.
func (p *ChocolateyPlugin) Validate(_ context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	parser := helpers.NewConfigParser(config)

	// Validate package_path.
	packagePath := parser.GetString("package_path", "", "")
	if packagePath == "" {
		vb.AddError("package_path", "package path is required")
	} else {
		// Only validate static parts (skip template validation).
		if !strings.Contains(packagePath, "{{") {
			if err := validatePackagePath(packagePath); err != nil {
				vb.AddError("package_path", err.Error())
			}
		} else {
			// Check basic format even with templates.
			if !strings.HasSuffix(strings.ToLower(packagePath), ".nupkg") {
				vb.AddError("package_path", "package path must end with .nupkg")
			}
		}
	}

	// Validate source URL if provided.
	source := parser.GetString("source", "", "https://push.chocolatey.org/")
	if source != "" && !strings.Contains(source, "{{") {
		if err := validateSourceURL(source); err != nil {
			vb.AddError("source", err.Error())
		}
	}

	// Validate timeout is positive.
	timeout := parser.GetInt("timeout", 300)
	if timeout <= 0 {
		vb.AddError("timeout", "timeout must be a positive integer")
	}

	return vb.Build(), nil
}

// parseConfig parses the plugin configuration with defaults and environment variable fallbacks.
func (p *ChocolateyPlugin) parseConfig(raw map[string]any) *Config {
	parser := helpers.NewConfigParser(raw)

	return &Config{
		APIKey:      parser.GetString("api_key", "CHOCOLATEY_API_KEY", ""),
		Source:      parser.GetString("source", "", "https://push.chocolatey.org/"),
		PackagePath: parser.GetString("package_path", "", ""),
		Timeout:     parser.GetInt("timeout", 300),
		Force:       parser.GetBool("force", false),
	}
}
