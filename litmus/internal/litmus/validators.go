package litmus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Validator struct {
	Type    string `json:"type"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

type ValidatorResult struct {
	Validator Validator `json:"validator"`
	Passed    bool      `json:"passed"`
	Error     bool      `json:"error,omitempty"`
	Reason    string    `json:"reason"`
}

var fencedCodePattern = regexp.MustCompile("(?s)```(?:python|py)?\\s*\\n?(.*?)```")

func runValidator(output, workspace string, validator Validator) ValidatorResult {
	result := ValidatorResult{Validator: validator}
	switch validator.Type {
	case "python_syntax":
		return validatePythonSyntax(output, result)
	case "python_tests":
		return validatePythonTests(output, workspace, validator, result)
	case "glide_batch_static":
		return validateGLIDEBatch(output, result)
	case "command", "workspace_command":
		return validateWorkspaceCommandResult(workspace, validator, result)
	default:
		result.Error = true
		result.Reason = fmt.Sprintf("unsupported validator type %q", validator.Type)
		return result
	}
}

var safeWorkspaceCommands = map[string]func([]string) bool{
	"go": func(args []string) bool {
		return len(args) > 0 && args[0] == "test"
	},
	"cargo": func(args []string) bool {
		return len(args) > 0 && args[0] == "test"
	},
	"pytest": func(_ []string) bool {
		return true
	},
	"python3": func(args []string) bool {
		return (len(args) >= 2 && args[0] == "-m" && args[1] == "pytest") ||
			(len(args) > 0 && strings.HasSuffix(args[0], ".py"))
	},
	"node": func(args []string) bool {
		if len(args) > 1 && args[0] == "--test" {
			return true
		}
		return len(args) > 0 &&
			(strings.HasSuffix(args[0], ".mjs") || strings.HasSuffix(args[0], ".js"))
	},
	"npm": func(args []string) bool {
		return safePackageScript(args)
	},
	"pnpm": func(args []string) bool {
		return safePackageScript(args)
	},
	"yarn": func(args []string) bool {
		return safePackageScript(args)
	},
}

var packageScriptPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)

func safePackageScript(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "test" {
		return true
	}
	return len(args) > 1 && args[0] == "run" && packageScriptPattern.MatchString(args[1])
}

func parseWorkspaceCommand(command string) ([]string, error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("workspace command is required")
	}
	if strings.ContainsAny(command, "\x00\r\n;&|`$<>(){}") {
		return nil, fmt.Errorf("workspace command must not contain shell syntax")
	}
	if strings.ContainsAny(command, `"'`) {
		return nil, fmt.Errorf("workspace command quoting is not supported; use simple arguments")
	}
	arguments := strings.Fields(command)
	if len(arguments) == 0 {
		return nil, fmt.Errorf("workspace command is required")
	}
	return arguments, nil
}

func validateWorkspaceCommand(command string) error {
	arguments, err := parseWorkspaceCommand(command)
	if err != nil {
		return err
	}
	if len(arguments) == 0 {
		return fmt.Errorf("workspace_command validator command is required")
	}
	executable := arguments[0]
	if executable == "" || filepath.Base(executable) != executable {
		return fmt.Errorf("workspace_command executable must be an allowed command name")
	}
	validateArgs, ok := safeWorkspaceCommands[executable]
	if !ok || !validateArgs(arguments[1:]) {
		return fmt.Errorf("workspace_command %q is not an allowed test command", command)
	}
	for _, argument := range arguments {
		if argument == "" || strings.ContainsAny(argument, "\x00\r\n") {
			return fmt.Errorf("workspace_command arguments must be non-empty single-line values")
		}
		if filepath.IsAbs(argument) {
			return fmt.Errorf("workspace_command arguments must not use absolute paths")
		}
		for _, component := range strings.FieldsFunc(argument, func(r rune) bool {
			return r == '/' || r == '\\'
		}) {
			if component == ".." {
				return fmt.Errorf("workspace_command arguments must not contain parent traversal")
			}
		}
	}
	return nil
}

func validateWorkspaceCommandResult(workspace string, validator Validator, result ValidatorResult) ValidatorResult {
	if err := validateWorkspaceCommand(validator.Command); err != nil {
		result.Error = true
		result.Reason = err.Error()
		return result
	}
	if strings.TrimSpace(workspace) == "" {
		result.Error = true
		result.Reason = "workspace_command validator requires a workspace"
		return result
	}
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		result.Error = true
		result.Reason = fmt.Sprintf("resolve workspace: %v", err)
		return result
	}
	arguments, err := parseWorkspaceCommand(validator.Command)
	if err != nil {
		result.Error = true
		result.Reason = err.Error()
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	command.Dir = resolvedWorkspace
	command.Env = append(os.Environ(), "CI=1")
	output, runErr := command.CombinedOutput()
	details := truncateCommandOutput(strings.TrimSpace(string(output)))
	if ctx.Err() != nil {
		result.Error = true
		result.Reason = fmt.Sprintf("workspace command timed out: %v", ctx.Err())
		return result
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			result.Error = true
			result.Reason = fmt.Sprintf("workspace command unavailable: %v", runErr)
			return result
		}
		result.Reason = fmt.Sprintf("workspace command failed: %v", runErr)
		if details != "" {
			result.Reason += ": " + details
		}
		return result
	}
	result.Passed = true
	result.Reason = "workspace command passed"
	if details != "" {
		result.Reason += ": " + details
	}
	return result
}

func truncateCommandOutput(output string) string {
	const limit = 4000
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "…"
}

func validatePythonSyntax(output string, result ValidatorResult) ValidatorResult {
	code := extractCode(output)
	if code == "" {
		result.Reason = "python syntax validator found no code"
		return result
	}
	available, reason := pythonAvailable()
	if !available {
		result.Error = true
		result.Reason = reason
		return result
	}

	path, cleanup, err := temporaryPythonFile(code)
	if err != nil {
		result.Error = true
		result.Reason = fmt.Sprintf("create syntax file: %v", err)
		return result
	}
	defer cleanup()

	_, runErr := runPython("-m", "py_compile", path)
	if runErr != nil {
		result.Reason = fmt.Sprintf("python syntax check failed: %v", runErr)
		return result
	}
	result.Passed = true
	result.Reason = "python syntax is valid"
	return result
}

func validatePythonTests(output, workspace string, validator Validator, result ValidatorResult) ValidatorResult {
	code := extractCode(output)
	if code == "" {
		result.Reason = "python test validator found no code"
		return result
	}
	if strings.TrimSpace(validator.Path) == "" {
		result.Error = true
		result.Reason = "python test validator requires a contract path"
		return result
	}
	contractPath, err := workspacePath(workspace, validator.Path)
	if err != nil {
		result.Error = true
		result.Reason = err.Error()
		return result
	}
	contract, err := os.ReadFile(contractPath)
	if err != nil {
		result.Error = true
		result.Reason = fmt.Sprintf("read python contract: %v", err)
		return result
	}
	available, reason := pythonAvailable()
	if !available {
		result.Error = true
		result.Reason = reason
		return result
	}
	pytestAvailable, reason := pytestAvailable()
	if !pytestAvailable {
		result.Error = true
		result.Reason = reason
		return result
	}

	path, cleanup, err := temporaryPythonFile(code + "\n\n" + string(contract))
	if err != nil {
		result.Error = true
		result.Reason = fmt.Sprintf("create python test file: %v", err)
		return result
	}
	defer cleanup()

	_, runErr := runPython("-m", "pytest", "-q", path)
	if runErr != nil {
		result.Reason = fmt.Sprintf("python contract failed: %v", runErr)
		return result
	}
	result.Passed = true
	result.Reason = "python contract passed"
	return result
}

func validateGLIDEBatch(output string, result ValidatorResult) ValidatorResult {
	code := extractCode(output)
	if code == "" {
		result.Reason = "GLIDE validator found no code"
		return result
	}
	lower := strings.ToLower(code)
	for _, forbidden := range []string{"asyncio.gather", "transaction", "clustertransaction"} {
		if strings.Contains(lower, forbidden) {
			result.Reason = fmt.Sprintf("deprecated or forbidden API %q found", forbidden)
			return result
		}
	}
	for _, required := range []string{"batch", ".exec(", "close", "try:", "finally:", "request_timeout"} {
		if !strings.Contains(lower, required) {
			result.Reason = fmt.Sprintf("required GLIDE batch pattern %q not found", required)
			return result
		}
	}
	if !strings.Contains(lower, "from glide") &&
		!strings.Contains(lower, "valkey-glide") &&
		!strings.Contains(lower, "valkey_glide") {
		result.Reason = "GLIDE import was not found"
		return result
	}
	result.Passed = true
	result.Reason = "GLIDE batch patterns are valid"
	return result
}

func extractCode(output string) string {
	matches := fencedCodePattern.FindStringSubmatch(output)
	if len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}
	return strings.TrimSpace(output)
}

func pythonAvailable() (bool, string) {
	if _, err := exec.LookPath("python3"); err != nil {
		return false, "validator_unavailable: python3 was not found"
	}
	return true, ""
}

func pytestAvailable() (bool, string) {
	_, err := runPython("-m", "pytest", "--version")
	if err != nil {
		return false, "validator_unavailable: pytest is not installed"
	}
	return true, ""
}

func runPython(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", args...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	if err != nil {
		return string(output), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func temporaryPythonFile(code string) (string, func(), error) {
	workspace, err := os.MkdirTemp("", "litmus-validator-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(workspace)
	}
	path := filepath.Join(workspace, "generated.py")
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}
