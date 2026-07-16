package litmus

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Validator struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
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
	default:
		result.Error = true
		result.Reason = fmt.Sprintf("unsupported validator type %q", validator.Type)
		return result
	}
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
