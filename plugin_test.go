// Package main provides tests for the Chocolatey plugin.
package main

import (
	"context"
	"os"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

func TestGetInfo(t *testing.T) {
	p := &ChocolateyPlugin{}
	info := p.GetInfo()

	if info.Name != "chocolatey" {
		t.Errorf("expected name 'chocolatey', got '%s'", info.Name)
	}

	if info.Version == "" {
		t.Error("expected non-empty version")
	}

	if info.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got '%s'", info.Version)
	}

	if info.Description == "" {
		t.Error("expected non-empty description")
	}

	if info.Author == "" {
		t.Error("expected non-empty author")
	}

	if info.Author != "Relicta Team" {
		t.Errorf("expected author 'Relicta Team', got '%s'", info.Author)
	}

	// Check hooks
	if len(info.Hooks) == 0 {
		t.Error("expected at least one hook")
	}

	hasPostPublish := false
	for _, hook := range info.Hooks {
		if hook == plugin.HookPostPublish {
			hasPostPublish = true
			break
		}
	}
	if !hasPostPublish {
		t.Error("expected PostPublish hook")
	}

	// Check config schema is valid JSON
	if info.ConfigSchema == "" {
		t.Error("expected non-empty config schema")
	}
}

func TestValidate(t *testing.T) {
	p := &ChocolateyPlugin{}
	ctx := context.Background()

	tests := []struct {
		name        string
		config      map[string]any
		wantValid   bool
		wantErrFld  string
		wantErrMsg  string
	}{
		{
			name:        "missing package_id",
			config:      map[string]any{},
			wantValid:   false,
			wantErrFld:  "package_id",
			wantErrMsg:  "Chocolatey package ID is required",
		},
		{
			name: "empty package_id",
			config: map[string]any{
				"package_id": "",
			},
			wantValid:  false,
			wantErrFld: "package_id",
			wantErrMsg: "Chocolatey package ID is required",
		},
		{
			name: "valid config with package_id",
			config: map[string]any{
				"package_id": "my-package",
			},
			wantValid: true,
		},
		{
			name: "valid config with all options",
			config: map[string]any{
				"package_id": "my-package",
				"api_key":    "secret-api-key",
				"source":     "https://custom.chocolatey.org/",
				"nuspec_dir": "./nuspec",
				"timeout":    600,
			},
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := p.Validate(ctx, tt.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Valid != tt.wantValid {
				t.Errorf("expected valid=%v, got valid=%v, errors=%v", tt.wantValid, resp.Valid, resp.Errors)
			}

			if !tt.wantValid && tt.wantErrFld != "" {
				if len(resp.Errors) == 0 {
					t.Error("expected validation errors, got none")
				} else {
					foundField := false
					for _, e := range resp.Errors {
						if e.Field == tt.wantErrFld {
							foundField = true
							if e.Message != tt.wantErrMsg {
								t.Errorf("expected error message '%s', got '%s'", tt.wantErrMsg, e.Message)
							}
							break
						}
					}
					if !foundField {
						t.Errorf("expected error on field '%s', got errors: %v", tt.wantErrFld, resp.Errors)
					}
				}
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	p := &ChocolateyPlugin{}

	tests := []struct {
		name     string
		config   map[string]any
		envVars  map[string]string
		expected Config
	}{
		{
			name:   "defaults",
			config: map[string]any{},
			expected: Config{
				PackageID: "",
				APIKey:    "",
				Source:    "https://push.chocolatey.org/",
				NuspecDir: ".",
				Timeout:   300,
			},
		},
		{
			name: "custom values",
			config: map[string]any{
				"package_id": "my-awesome-package",
				"api_key":    "my-secret-key",
				"source":     "https://custom.chocolatey.org/",
				"nuspec_dir": "./packages",
				"timeout":    600,
			},
			expected: Config{
				PackageID: "my-awesome-package",
				APIKey:    "my-secret-key",
				Source:    "https://custom.chocolatey.org/",
				NuspecDir: "./packages",
				Timeout:   600,
			},
		},
		{
			name:   "env var fallback for api_key",
			config: map[string]any{},
			envVars: map[string]string{
				"CHOCOLATEY_API_KEY": "env-api-key",
			},
			expected: Config{
				PackageID: "",
				APIKey:    "env-api-key",
				Source:    "https://push.chocolatey.org/",
				NuspecDir: ".",
				Timeout:   300,
			},
		},
		{
			name: "config value takes precedence over env var",
			config: map[string]any{
				"api_key": "config-api-key",
			},
			envVars: map[string]string{
				"CHOCOLATEY_API_KEY": "env-api-key",
			},
			expected: Config{
				PackageID: "",
				APIKey:    "config-api-key",
				Source:    "https://push.chocolatey.org/",
				NuspecDir: ".",
				Timeout:   300,
			},
		},
		{
			name: "partial config with some defaults",
			config: map[string]any{
				"package_id": "partial-package",
			},
			expected: Config{
				PackageID: "partial-package",
				APIKey:    "",
				Source:    "https://push.chocolatey.org/",
				NuspecDir: ".",
				Timeout:   300,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear relevant env vars first
			os.Unsetenv("CHOCOLATEY_API_KEY")

			// Set env vars for this test
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			cfg := p.parseConfig(tt.config)

			if cfg.PackageID != tt.expected.PackageID {
				t.Errorf("PackageID: expected '%s', got '%s'", tt.expected.PackageID, cfg.PackageID)
			}
			if cfg.APIKey != tt.expected.APIKey {
				t.Errorf("APIKey: expected '%s', got '%s'", tt.expected.APIKey, cfg.APIKey)
			}
			if cfg.Source != tt.expected.Source {
				t.Errorf("Source: expected '%s', got '%s'", tt.expected.Source, cfg.Source)
			}
			if cfg.NuspecDir != tt.expected.NuspecDir {
				t.Errorf("NuspecDir: expected '%s', got '%s'", tt.expected.NuspecDir, cfg.NuspecDir)
			}
			if cfg.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout: expected %d, got %d", tt.expected.Timeout, cfg.Timeout)
			}
		})
	}
}

func TestExecuteDryRun(t *testing.T) {
	p := &ChocolateyPlugin{}
	ctx := context.Background()

	tests := []struct {
		name           string
		config         map[string]any
		releaseCtx     plugin.ReleaseContext
		expectedMsg    string
		expectedResult bool
	}{
		{
			name: "basic dry run execution",
			config: map[string]any{
				"package_id": "my-package",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.2.3",
			},
			expectedMsg:    "Would execute chocolatey plugin",
			expectedResult: true,
		},
		{
			name: "dry run with all options",
			config: map[string]any{
				"package_id": "my-awesome-package",
				"api_key":    "test-key",
				"source":     "https://custom.chocolatey.org/",
			},
			releaseCtx: plugin.ReleaseContext{
				Version:  "v2.0.0",
				Branch:   "main",
				TagName:  "v2.0.0",
				CommitSHA: "abc123",
			},
			expectedMsg:    "Would execute chocolatey plugin",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook:    plugin.HookPostPublish,
				Config:  tt.config,
				Context: tt.releaseCtx,
				DryRun:  true,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Success != tt.expectedResult {
				t.Errorf("expected success=%v, got success=%v, error: %s", tt.expectedResult, resp.Success, resp.Error)
			}

			if resp.Message != tt.expectedMsg {
				t.Errorf("expected message '%s', got '%s'", tt.expectedMsg, resp.Message)
			}
		})
	}
}

func TestExecuteUnhandledHook(t *testing.T) {
	p := &ChocolateyPlugin{}
	ctx := context.Background()

	tests := []struct {
		name string
		hook plugin.Hook
	}{
		{
			name: "PreInit hook",
			hook: plugin.HookPreInit,
		},
		{
			name: "PostInit hook",
			hook: plugin.HookPostInit,
		},
		{
			name: "PrePlan hook",
			hook: plugin.HookPrePlan,
		},
		{
			name: "PostPlan hook",
			hook: plugin.HookPostPlan,
		},
		{
			name: "PreVersion hook",
			hook: plugin.HookPreVersion,
		},
		{
			name: "PostVersion hook",
			hook: plugin.HookPostVersion,
		},
		{
			name: "PreNotes hook",
			hook: plugin.HookPreNotes,
		},
		{
			name: "PostNotes hook",
			hook: plugin.HookPostNotes,
		},
		{
			name: "PreApprove hook",
			hook: plugin.HookPreApprove,
		},
		{
			name: "PostApprove hook",
			hook: plugin.HookPostApprove,
		},
		{
			name: "PrePublish hook",
			hook: plugin.HookPrePublish,
		},
		{
			name: "OnSuccess hook",
			hook: plugin.HookOnSuccess,
		},
		{
			name: "OnError hook",
			hook: plugin.HookOnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook: tt.hook,
				Config: map[string]any{
					"package_id": "test-package",
				},
				DryRun: true,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !resp.Success {
				t.Error("expected success for unhandled hook")
			}

			expectedMsg := "Hook " + string(tt.hook) + " not handled"
			if resp.Message != expectedMsg {
				t.Errorf("expected message '%s', got '%s'", expectedMsg, resp.Message)
			}
		})
	}
}
