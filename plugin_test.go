// Package main provides tests for the Chocolatey plugin.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// MockCommandExecutor records and simulates command execution for testing.
type MockCommandExecutor struct {
	// Commands records all executed commands.
	Commands []ExecutedCommand
	// Output is the output to return from Run.
	Output []byte
	// Err is the error to return from Run.
	Err error
}

// ExecutedCommand represents a recorded command execution.
type ExecutedCommand struct {
	Name string
	Args []string
}

// Run records the command and returns the configured output/error.
func (m *MockCommandExecutor) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.Commands = append(m.Commands, ExecutedCommand{Name: name, Args: args})
	return m.Output, m.Err
}

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

	// Check hooks.
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

	// Check config schema is valid JSON.
	if info.ConfigSchema == "" {
		t.Error("expected non-empty config schema")
	}

	// Verify schema contains expected fields.
	if !strings.Contains(info.ConfigSchema, "api_key") {
		t.Error("config schema should contain api_key field")
	}
	if !strings.Contains(info.ConfigSchema, "package_path") {
		t.Error("config schema should contain package_path field")
	}
	if !strings.Contains(info.ConfigSchema, "source") {
		t.Error("config schema should contain source field")
	}
	if !strings.Contains(info.ConfigSchema, "timeout") {
		t.Error("config schema should contain timeout field")
	}
	if !strings.Contains(info.ConfigSchema, "force") {
		t.Error("config schema should contain force field")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]any
		wantValid  bool
		wantErrFld string
		wantErrMsg string
	}{
		{
			name:       "missing package_path",
			config:     map[string]any{},
			wantValid:  false,
			wantErrFld: "package_path",
			wantErrMsg: "package path is required",
		},
		{
			name: "empty package_path",
			config: map[string]any{
				"package_path": "",
			},
			wantValid:  false,
			wantErrFld: "package_path",
			wantErrMsg: "package path is required",
		},
		{
			name: "invalid package_path - not nupkg",
			config: map[string]any{
				"package_path": "mypackage.zip",
			},
			wantValid:  false,
			wantErrFld: "package_path",
			wantErrMsg: "package path must end with .nupkg",
		},
		{
			name: "invalid package_path - path traversal",
			config: map[string]any{
				"package_path": "../../../etc/passwd.nupkg",
			},
			wantValid:  false,
			wantErrFld: "package_path",
			wantErrMsg: "path traversal detected: cannot use '..' to escape working directory",
		},
		{
			name: "valid package_path with template",
			config: map[string]any{
				"package_path": "mypackage.{{version}}.nupkg",
			},
			wantValid: true,
		},
		{
			name: "valid package_path",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
			},
			wantValid: true,
		},
		{
			name: "valid config with all options",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "secret-api-key",
				"source":       "https://push.chocolatey.org/",
				"timeout":      600,
				"force":        true,
			},
			wantValid: true,
		},
		{
			name: "invalid source - not https",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"source":       "http://push.chocolatey.org/",
			},
			wantValid:  false,
			wantErrFld: "source",
			wantErrMsg: "only HTTPS URLs are allowed (got http)",
		},
		{
			name: "valid source - localhost http allowed",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"source":       "http://localhost:8080/",
			},
			wantValid: true,
		},
		{
			name: "invalid timeout - zero",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"timeout":      0,
			},
			wantValid:  false,
			wantErrFld: "timeout",
			wantErrMsg: "timeout must be a positive integer",
		},
		{
			name: "invalid timeout - negative",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"timeout":      -1,
			},
			wantValid:  false,
			wantErrFld: "timeout",
			wantErrMsg: "timeout must be a positive integer",
		},
	}

	p := &ChocolateyPlugin{}
	ctx := context.Background()

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
				APIKey:      "",
				Source:      "https://push.chocolatey.org/",
				PackagePath: "",
				Timeout:     300,
				Force:       false,
			},
		},
		{
			name: "custom values",
			config: map[string]any{
				"api_key":      "my-secret-key",
				"source":       "https://custom.chocolatey.org/",
				"package_path": "mypackage.1.0.0.nupkg",
				"timeout":      600,
				"force":        true,
			},
			expected: Config{
				APIKey:      "my-secret-key",
				Source:      "https://custom.chocolatey.org/",
				PackagePath: "mypackage.1.0.0.nupkg",
				Timeout:     600,
				Force:       true,
			},
		},
		{
			name:   "env var fallback for api_key",
			config: map[string]any{},
			envVars: map[string]string{
				"CHOCOLATEY_API_KEY": "env-api-key",
			},
			expected: Config{
				APIKey:      "env-api-key",
				Source:      "https://push.chocolatey.org/",
				PackagePath: "",
				Timeout:     300,
				Force:       false,
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
				APIKey:      "config-api-key",
				Source:      "https://push.chocolatey.org/",
				PackagePath: "",
				Timeout:     300,
				Force:       false,
			},
		},
		{
			name: "partial config with some defaults",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"force":        true,
			},
			expected: Config{
				APIKey:      "",
				Source:      "https://push.chocolatey.org/",
				PackagePath: "mypackage.1.0.0.nupkg",
				Timeout:     300,
				Force:       true,
			},
		},
	}

	p := &ChocolateyPlugin{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear relevant env vars first.
			_ = os.Unsetenv("CHOCOLATEY_API_KEY")

			// Set env vars for this test.
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
				defer func(key string) { _ = os.Unsetenv(key) }(k)
			}

			cfg := p.parseConfig(tt.config)

			if cfg.APIKey != tt.expected.APIKey {
				t.Errorf("APIKey: expected '%s', got '%s'", tt.expected.APIKey, cfg.APIKey)
			}
			if cfg.Source != tt.expected.Source {
				t.Errorf("Source: expected '%s', got '%s'", tt.expected.Source, cfg.Source)
			}
			if cfg.PackagePath != tt.expected.PackagePath {
				t.Errorf("PackagePath: expected '%s', got '%s'", tt.expected.PackagePath, cfg.PackagePath)
			}
			if cfg.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout: expected %d, got %d", tt.expected.Timeout, cfg.Timeout)
			}
			if cfg.Force != tt.expected.Force {
				t.Errorf("Force: expected %v, got %v", tt.expected.Force, cfg.Force)
			}
		})
	}
}

func TestExecuteDryRun(t *testing.T) {
	tests := []struct {
		name           string
		config         map[string]any
		releaseCtx     plugin.ReleaseContext
		expectedMsg    string
		expectedResult bool
		checkOutputs   func(t *testing.T, outputs map[string]any)
	}{
		{
			name: "basic dry run execution",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-key",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.2.3",
			},
			expectedMsg:    "Would push Chocolatey package",
			expectedResult: true,
			checkOutputs: func(t *testing.T, outputs map[string]any) {
				if outputs["package_path"] != "mypackage.1.0.0.nupkg" {
					t.Errorf("expected package_path 'mypackage.1.0.0.nupkg', got '%v'", outputs["package_path"])
				}
				if outputs["version"] != "1.2.3" {
					t.Errorf("expected version '1.2.3', got '%v'", outputs["version"])
				}
			},
		},
		{
			name: "dry run with version template",
			config: map[string]any{
				"package_path": "mypackage.{{version}}.nupkg",
				"api_key":      "test-key",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v2.0.0",
			},
			expectedMsg:    "Would push Chocolatey package",
			expectedResult: true,
			checkOutputs: func(t *testing.T, outputs map[string]any) {
				if outputs["package_path"] != "mypackage.2.0.0.nupkg" {
					t.Errorf("expected package_path 'mypackage.2.0.0.nupkg', got '%v'", outputs["package_path"])
				}
			},
		},
		{
			name: "dry run with all options",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-key",
				"source":       "http://localhost:8080/",
				"timeout":      600,
				"force":        true,
			},
			releaseCtx: plugin.ReleaseContext{
				Version:   "v2.0.0",
				Branch:    "main",
				TagName:   "v2.0.0",
				CommitSHA: "abc123",
			},
			expectedMsg:    "Would push Chocolatey package",
			expectedResult: true,
			checkOutputs: func(t *testing.T, outputs map[string]any) {
				if outputs["source"] != "http://localhost:8080/" {
					t.Errorf("expected source 'http://localhost:8080/', got '%v'", outputs["source"])
				}
				if outputs["force"] != true {
					t.Errorf("expected force true, got '%v'", outputs["force"])
				}
				if outputs["timeout"] != 600 {
					t.Errorf("expected timeout 600, got '%v'", outputs["timeout"])
				}
			},
		},
	}

	p := &ChocolateyPlugin{}
	ctx := context.Background()

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

			if tt.checkOutputs != nil && resp.Outputs != nil {
				tt.checkOutputs(t, resp.Outputs)
			}
		})
	}
}

func TestExecuteWithMockExecutor(t *testing.T) {
	tests := []struct {
		name           string
		config         map[string]any
		releaseCtx     plugin.ReleaseContext
		mockOutput     []byte
		mockErr        error
		expectedResult bool
		expectedError  string
		checkCommand   func(t *testing.T, cmds []ExecutedCommand)
	}{
		{
			name: "successful push",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-api-key",
				"source":       "https://push.chocolatey.org/",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockOutput:     []byte("Package pushed successfully"),
			mockErr:        nil,
			expectedResult: true,
			checkCommand: func(t *testing.T, cmds []ExecutedCommand) {
				if len(cmds) != 1 {
					t.Fatalf("expected 1 command, got %d", len(cmds))
				}
				cmd := cmds[0]
				if cmd.Name != "choco" {
					t.Errorf("expected 'choco', got '%s'", cmd.Name)
				}
				expectedArgs := []string{"push", "mypackage.1.0.0.nupkg", "--api-key", "test-api-key", "--source", "https://push.chocolatey.org/", "--timeout", "300"}
				if len(cmd.Args) != len(expectedArgs) {
					t.Errorf("expected %d args, got %d: %v", len(expectedArgs), len(cmd.Args), cmd.Args)
				}
				for i, arg := range expectedArgs {
					if i < len(cmd.Args) && cmd.Args[i] != arg {
						t.Errorf("arg %d: expected '%s', got '%s'", i, arg, cmd.Args[i])
					}
				}
			},
		},
		{
			name: "push with force flag",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-api-key",
				"force":        true,
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockOutput:     []byte("Package pushed successfully"),
			mockErr:        nil,
			expectedResult: true,
			checkCommand: func(t *testing.T, cmds []ExecutedCommand) {
				if len(cmds) != 1 {
					t.Fatalf("expected 1 command, got %d", len(cmds))
				}
				cmd := cmds[0]
				hasForce := false
				for _, arg := range cmd.Args {
					if arg == "--force" {
						hasForce = true
						break
					}
				}
				if !hasForce {
					t.Error("expected --force flag in command args")
				}
			},
		},
		{
			name: "push with custom timeout",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-api-key",
				"timeout":      600,
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockOutput:     []byte("Package pushed successfully"),
			mockErr:        nil,
			expectedResult: true,
			checkCommand: func(t *testing.T, cmds []ExecutedCommand) {
				if len(cmds) != 1 {
					t.Fatalf("expected 1 command, got %d", len(cmds))
				}
				cmd := cmds[0]
				foundTimeout := false
				for i, arg := range cmd.Args {
					if arg == "--timeout" && i+1 < len(cmd.Args) {
						if cmd.Args[i+1] == "600" {
							foundTimeout = true
						}
						break
					}
				}
				if !foundTimeout {
					t.Error("expected --timeout 600 in command args")
				}
			},
		},
		{
			name: "push with version template",
			config: map[string]any{
				"package_path": "mypackage.{{version}}.nupkg",
				"api_key":      "test-api-key",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v2.1.0",
			},
			mockOutput:     []byte("Package pushed successfully"),
			mockErr:        nil,
			expectedResult: true,
			checkCommand: func(t *testing.T, cmds []ExecutedCommand) {
				if len(cmds) != 1 {
					t.Fatalf("expected 1 command, got %d", len(cmds))
				}
				cmd := cmds[0]
				if cmd.Args[1] != "mypackage.2.1.0.nupkg" {
					t.Errorf("expected 'mypackage.2.1.0.nupkg', got '%s'", cmd.Args[1])
				}
			},
		},
		{
			name: "push failure",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-api-key",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockOutput:     []byte("Error: Package already exists"),
			mockErr:        fmt.Errorf("exit status 1"),
			expectedResult: false,
			expectedError:  "choco push failed",
		},
		{
			name: "missing api key",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			expectedResult: false,
			expectedError:  "API key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommandExecutor{
				Output: tt.mockOutput,
				Err:    tt.mockErr,
			}
			p := &ChocolateyPlugin{cmdExecutor: mock}
			ctx := context.Background()

			req := plugin.ExecuteRequest{
				Hook:    plugin.HookPostPublish,
				Config:  tt.config,
				Context: tt.releaseCtx,
				DryRun:  false,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Success != tt.expectedResult {
				t.Errorf("expected success=%v, got success=%v, error: %s", tt.expectedResult, resp.Success, resp.Error)
			}

			if tt.expectedError != "" && !strings.Contains(resp.Error, tt.expectedError) {
				t.Errorf("expected error containing '%s', got '%s'", tt.expectedError, resp.Error)
			}

			if tt.checkCommand != nil {
				tt.checkCommand(t, mock.Commands)
			}
		})
	}
}

func TestExecuteUnhandledHook(t *testing.T) {
	tests := []struct {
		name string
		hook plugin.Hook
	}{
		{name: "PreInit hook", hook: plugin.HookPreInit},
		{name: "PostInit hook", hook: plugin.HookPostInit},
		{name: "PrePlan hook", hook: plugin.HookPrePlan},
		{name: "PostPlan hook", hook: plugin.HookPostPlan},
		{name: "PreVersion hook", hook: plugin.HookPreVersion},
		{name: "PostVersion hook", hook: plugin.HookPostVersion},
		{name: "PreNotes hook", hook: plugin.HookPreNotes},
		{name: "PostNotes hook", hook: plugin.HookPostNotes},
		{name: "PreApprove hook", hook: plugin.HookPreApprove},
		{name: "PostApprove hook", hook: plugin.HookPostApprove},
		{name: "PrePublish hook", hook: plugin.HookPrePublish},
		{name: "OnSuccess hook", hook: plugin.HookOnSuccess},
		{name: "OnError hook", hook: plugin.HookOnError},
	}

	p := &ChocolateyPlugin{}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook: tt.hook,
				Config: map[string]any{
					"package_path": "test.nupkg",
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

func TestValidatePackagePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty path",
			path:      "",
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
		{
			name:    "valid simple path",
			path:    "mypackage.1.0.0.nupkg",
			wantErr: false,
		},
		{
			name:    "valid path with directory",
			path:    "packages/mypackage.1.0.0.nupkg",
			wantErr: false,
		},
		{
			name:      "path traversal - parent directory",
			path:      "../mypackage.1.0.0.nupkg",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:      "path traversal - deep",
			path:      "packages/../../etc/passwd.nupkg",
			wantErr:   true,
			errSubstr: "path traversal",
		},
		{
			name:      "wrong extension",
			path:      "mypackage.1.0.0.zip",
			wantErr:   true,
			errSubstr: "must end with .nupkg",
		},
		{
			name:    "case insensitive extension",
			path:    "mypackage.1.0.0.NUPKG",
			wantErr: false,
		},
		{
			name:    "path too long",
			path:    strings.Repeat("a", 510) + ".nupkg",
			wantErr: true,
		},
		{
			name:      "invalid characters",
			path:      "my;package.nupkg",
			wantErr:   true,
			errSubstr: "disallowed characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePackagePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateSourceURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty URL",
			url:       "",
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
		{
			name:    "valid HTTPS URL",
			url:     "https://push.chocolatey.org/",
			wantErr: false,
		},
		{
			name:      "HTTP URL not allowed",
			url:       "http://push.chocolatey.org/",
			wantErr:   true,
			errSubstr: "only HTTPS URLs are allowed",
		},
		{
			name:    "localhost HTTP allowed",
			url:     "http://localhost:8080/",
			wantErr: false,
		},
		{
			name:    "localhost HTTPS allowed",
			url:     "https://localhost:8080/",
			wantErr: false,
		},
		{
			name:    "127.0.0.1 HTTP allowed",
			url:     "http://127.0.0.1:8080/",
			wantErr: false,
		},
		{
			name:      "invalid URL",
			url:       "not-a-url",
			wantErr:   true,
			errSubstr: "only HTTPS URLs are allowed",
		},
		{
			name:      "file URL not allowed",
			url:       "file:///etc/passwd",
			wantErr:   true,
			errSubstr: "only HTTPS URLs are allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSourceURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{name: "private 10.x.x.x", ip: "10.0.0.1", expected: true},
		{name: "private 172.16.x.x", ip: "172.16.0.1", expected: true},
		{name: "private 192.168.x.x", ip: "192.168.1.1", expected: true},
		{name: "loopback", ip: "127.0.0.1", expected: true},
		{name: "cloud metadata", ip: "169.254.169.254", expected: true},
		{name: "link-local", ip: "169.254.1.1", expected: true},
		{name: "public IP", ip: "8.8.8.8", expected: false},
		{name: "public IP 2", ip: "1.1.1.1", expected: false},
		{name: "IPv6 loopback", ip: "::1", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}
			result := isPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// parseIP parses an IP address string for testing.
func parseIP(s string) []byte {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return parseIPv4(s)
		}
		if s[i] == ':' {
			return parseIPv6(s)
		}
	}
	return nil
}

func parseIPv4(s string) []byte {
	var ip [4]byte
	var n, val int
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			ip[n] = byte(val)
			n++
			val = 0
		} else {
			val = val*10 + int(s[i]-'0')
		}
	}
	ip[n] = byte(val)
	return ip[:]
}

func parseIPv6(s string) []byte {
	// Simplified parsing for common cases.
	if s == "::1" {
		return []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	}
	return nil
}

func TestBuildPushArgs(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		packagePath string
		expected    []string
	}{
		{
			name: "basic args",
			cfg: &Config{
				APIKey:  "test-key",
				Source:  "https://push.chocolatey.org/",
				Timeout: 300,
				Force:   false,
			},
			packagePath: "mypackage.1.0.0.nupkg",
			expected:    []string{"push", "mypackage.1.0.0.nupkg", "--api-key", "test-key", "--source", "https://push.chocolatey.org/", "--timeout", "300"},
		},
		{
			name: "with force flag",
			cfg: &Config{
				APIKey:  "test-key",
				Source:  "https://push.chocolatey.org/",
				Timeout: 300,
				Force:   true,
			},
			packagePath: "mypackage.1.0.0.nupkg",
			expected:    []string{"push", "mypackage.1.0.0.nupkg", "--api-key", "test-key", "--source", "https://push.chocolatey.org/", "--timeout", "300", "--force"},
		},
		{
			name: "custom timeout",
			cfg: &Config{
				APIKey:  "test-key",
				Source:  "https://custom.org/",
				Timeout: 600,
				Force:   false,
			},
			packagePath: "mypackage.1.0.0.nupkg",
			expected:    []string{"push", "mypackage.1.0.0.nupkg", "--api-key", "test-key", "--source", "https://custom.org/", "--timeout", "600"},
		},
	}

	p := &ChocolateyPlugin{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := p.buildPushArgs(tt.cfg, tt.packagePath)
			if len(args) != len(tt.expected) {
				t.Errorf("expected %d args, got %d: %v", len(tt.expected), len(args), args)
				return
			}
			for i, arg := range tt.expected {
				if args[i] != arg {
					t.Errorf("arg %d: expected '%s', got '%s'", i, arg, args[i])
				}
			}
		})
	}
}

func TestGetExecutor(t *testing.T) {
	t.Run("returns custom executor when set", func(t *testing.T) {
		mock := &MockCommandExecutor{}
		p := &ChocolateyPlugin{cmdExecutor: mock}
		executor := p.getExecutor()
		if executor != mock {
			t.Error("expected custom executor to be returned")
		}
	})

	t.Run("returns RealCommandExecutor when nil", func(t *testing.T) {
		p := &ChocolateyPlugin{}
		executor := p.getExecutor()
		if _, ok := executor.(*RealCommandExecutor); !ok {
			t.Error("expected RealCommandExecutor to be returned")
		}
	})
}

func TestExecuteValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		config        map[string]any
		releaseCtx    plugin.ReleaseContext
		expectedError string
	}{
		{
			name: "invalid package path - path traversal",
			config: map[string]any{
				"package_path": "../../../malicious.nupkg",
				"api_key":      "test-key",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			expectedError: "invalid package path",
		},
		{
			name: "invalid source URL - not HTTPS",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
				"api_key":      "test-key",
				"source":       "http://evil.com/",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			expectedError: "invalid source URL",
		},
		{
			name: "missing API key",
			config: map[string]any{
				"package_path": "mypackage.1.0.0.nupkg",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			expectedError: "API key is required",
		},
	}

	p := &ChocolateyPlugin{}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook:    plugin.HookPostPublish,
				Config:  tt.config,
				Context: tt.releaseCtx,
				DryRun:  false,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Success {
				t.Error("expected failure for invalid config")
			}

			if !strings.Contains(resp.Error, tt.expectedError) {
				t.Errorf("expected error containing '%s', got '%s'", tt.expectedError, resp.Error)
			}
		})
	}
}
