package litmus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidatePythonSyntaxRejectsInvalidSnippet(t *testing.T) {
	result := runValidator("```python\nif:\n```", t.TempDir(), Validator{
		Type: "python_syntax",
	})
	if result.Passed || result.Error || !strings.Contains(result.Reason, "syntax") {
		t.Fatalf("runValidator() = %#v, want syntax failure", result)
	}
}

func TestValidatePythonTestsRequiresExpectedBehavior(t *testing.T) {
	workspace := t.TempDir()
	writeValidatorFile(t, filepath.Join(workspace, "contract.py"), `
def test_parse_pair_contract():
    assert parse_pair("orphan_key") == ("orphan_key", None)
    assert parse_pair("key=value") == ("key", "value")
`)
	result := runValidator("```python\n"+
		"def parse_pair(s):\n"+
		"    if '=' not in s:\n"+
		"        return s.strip(), None\n"+
		"    key, value = s.split('=', 1)\n"+
		"    return key.strip(), value.strip()\n"+
		"```", workspace, Validator{
		Type: "python_tests",
		Path: "contract.py",
	})
	if !result.Passed || result.Error {
		t.Fatalf("runValidator() = %#v, want contract pass", result)
	}
}

func TestValidateGeneratedBatchRejectsDeprecatedTransactionAPI(t *testing.T) {
	result := runValidator("```python\nfrom glide import Transaction\nawait client.exec(Transaction())\n```", t.TempDir(), Validator{
		Type: "glide_batch_static",
	})
	if result.Passed || result.Error || !strings.Contains(result.Reason, "deprecated") {
		t.Fatalf("runValidator() = %#v, want deprecated API failure", result)
	}
}

func TestWorkspaceCommandValidatorRunsAllowedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a Unix executable")
	}
	workspace := t.TempDir()
	fakeBin := t.TempDir()
	writeValidatorExecutable(t, filepath.Join(fakeBin, "go"), `#!/bin/sh
test "$1" = "test"
test "$2" = "./..."
test "$(pwd)" = "$EXPECTED_WORKSPACE"
`)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXPECTED_WORKSPACE", resolvedWorkspace)

	result := runValidator("", workspace, Validator{
		Type:    "command",
		Command: "go test ./...",
	})
	if !result.Passed || result.Error {
		t.Fatalf("runValidator() = %#v, want workspace command pass", result)
	}
}

func TestWorkspaceCommandValidatorRunsWorkspaceRelativeNodeCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a Unix executable")
	}
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "checks", "check-doc.mjs"), "// fixture check")
	fakeBin := t.TempDir()
	writeValidatorExecutable(t, filepath.Join(fakeBin, "node"), `#!/bin/sh
test "$1" = "checks/check-doc.mjs"
test "$2" = "docs/quickstart.md"
test "$3" = "quickstart"
`)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result := runValidator("", workspace, Validator{
		Type:    "command",
		Command: "node checks/check-doc.mjs docs/quickstart.md quickstart",
	})
	if !result.Passed || result.Error {
		t.Fatalf("runValidator() = %#v, want workspace-relative Node check pass", result)
	}
}

func TestWorkspaceCommandValidatorRejectsUnsafeCommands(t *testing.T) {
	for _, command := range []string{
		"",
		"sh -c go-test",
		"go run main.go",
		"python3 -c print",
		"go test ../outside",
		"npm run test;rm",
		"node -e process.exit(0)",
		"node ../checks/check-doc.mjs",
		"node /tmp/check-doc.mjs",
	} {
		result := runValidator("", t.TempDir(), Validator{
			Type:    "command",
			Command: command,
		})
		if result.Passed || !result.Error {
			t.Fatalf("runValidator(%#v) = %#v, want grader error", command, result)
		}
	}
}

func TestWorkspaceCommandValidatorClassifiesTestFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a Unix executable")
	}
	fakeBin := t.TempDir()
	writeValidatorExecutable(t, filepath.Join(fakeBin, "pytest"), "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result := runValidator("", t.TempDir(), Validator{
		Type:    "command",
		Command: "pytest -q",
	})
	if result.Passed || result.Error || !strings.Contains(result.Reason, "failed") {
		t.Fatalf("runValidator() = %#v, want agent test failure", result)
	}
}

func writeValidatorFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeValidatorExecutable(t *testing.T, path, contents string) {
	t.Helper()
	writeValidatorFile(t, path, contents)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
