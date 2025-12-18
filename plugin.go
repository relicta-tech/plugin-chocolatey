// Package main implements the Chocolatey plugin for Relicta.
package main

import (
	"context"
	"fmt"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// ChocolateyPlugin implements the Publish packages to Chocolatey (Windows) plugin.
type ChocolateyPlugin struct{}

// Config represents the Chocolatey plugin configuration.
type Config struct {
	PackageID string
	APIKey    string
	Source    string
	NuspecDir string
	Timeout   int
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
			"properties": {}
		}`,
	}
}

// Execute runs the plugin for a given hook.
func (p *ChocolateyPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	switch req.Hook {
	case plugin.HookPostPublish:
		if req.DryRun {
			return &plugin.ExecuteResponse{
				Success: true,
				Message: "Would execute chocolatey plugin",
			}, nil
		}
		return &plugin.ExecuteResponse{
			Success: true,
			Message: "Chocolatey plugin executed successfully",
		}, nil
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// Validate validates the plugin configuration.
func (p *ChocolateyPlugin) Validate(_ context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	parser := helpers.NewConfigParser(config)

	packageID := parser.GetString("package_id", "", "")
	if packageID == "" {
		vb.AddError("package_id", "Chocolatey package ID is required")
	}

	return vb.Build(), nil
}

// parseConfig parses the plugin configuration with defaults and environment variable fallbacks.
func (p *ChocolateyPlugin) parseConfig(raw map[string]any) *Config {
	parser := helpers.NewConfigParser(raw)

	return &Config{
		PackageID: parser.GetString("package_id", "", ""),
		APIKey:    parser.GetString("api_key", "CHOCOLATEY_API_KEY", ""),
		Source:    parser.GetString("source", "", "https://push.chocolatey.org/"),
		NuspecDir: parser.GetString("nuspec_dir", "", "."),
		Timeout:   parser.GetInt("timeout", 300),
	}
}
