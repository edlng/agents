package litmus

import (
	"os"
	"path/filepath"
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

func writeValidatorFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
