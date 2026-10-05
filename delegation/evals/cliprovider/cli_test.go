package cliprovider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
)

// fakeCLI writes a script that records its arguments and stdin, then prints out.
func fakeCLI(t *testing.T, out string) (binary, argsFile, stdinFile string) {
	dir := t.TempDir()
	argsFile, stdinFile = filepath.Join(dir, "args"), filepath.Join(dir, "stdin")
	binary = filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat > " + stdinFile + "\ncat <<'JSON'\n" + out + "\nJSON\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, argsFile, stdinFile
}

func TestToolCallFromStructuredOutput(t *testing.T) {
	binary, argsFile, stdinFile := fakeCLI(t, `{"is_error":false,"session_id":"sess","total_cost_usd":0.004,"duration_ms":900,
		"structured_output":{"text":"adding","tool_calls":[{"name":"add","input":{"a":2,"b":3}}]},
		"modelUsage":{"claude-sonnet-5-5":{"inputTokens":40,"outputTokens":12,"cacheReadInputTokens":0,"cacheCreationInputTokens":1100}}}`)
	req := provider.Request{
		Model:  "claude-sonnet-5-5",
		System: "be a calculator",
		Messages: []provider.Message{
			provider.UserText("2+3?"),
		},
		Tools:     []provider.Tool{{Name: "add", Description: "add", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		MaxTokens: 100,
	}
	resp, err := CLI{Binary: binary}.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	calls := resp.ToolCalls()
	if resp.StopReason != "tool_use" || len(calls) != 1 || calls[0].Name != "add" || string(calls[0].Input) != `{"a":2,"b":3}` || calls[0].ID == "" {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Usage.CostUSD != 0.004 || resp.Usage.CacheWriteTokens != 1100 || resp.Model != "claude-sonnet-5-5" {
		t.Fatalf("usage = %+v model = %s", resp.Usage, resp.Model)
	}
	args, _ := os.ReadFile(argsFile)
	for _, want := range []string{"--safe-mode", "--tools\n\n", `"enum":["add"]`, "## Harness tools"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args missing %q", want)
		}
	}
	stdin, _ := os.ReadFile(stdinFile)
	if !strings.Contains(string(stdin), "<user>\n2+3?\n</user>") {
		t.Errorf("stdin = %q", stdin)
	}
}

func TestRejectsSilentModelFallback(t *testing.T) {
	binary, _, _ := fakeCLI(t, `{"is_error":false,"structured_output":{"text":"hi","tool_calls":[]},
		"modelUsage":{"global.anthropic.claude-opus-5[1m]":{"inputTokens":1,"outputTokens":1}}}`)
	_, err := CLI{Binary: binary}.Complete(context.Background(), provider.Request{Model: "claude-haiku-4-5", Messages: []provider.Message{provider.UserText("hi")}})
	if err == nil || !strings.Contains(err.Error(), "CLI ran global.anthropic.claude-opus-5[1m]") {
		t.Fatalf("err = %v", err)
	}
}

func TestReportsCLIError(t *testing.T) {
	binary, _, _ := fakeCLI(t, `{"is_error":true,"result":"API Error: Could not load AWS credentials"}`)
	_, err := CLI{Binary: binary}.Complete(context.Background(), provider.Request{Model: "claude-sonnet-5-5"})
	if err == nil || !strings.Contains(err.Error(), "AWS credentials") {
		t.Fatalf("err = %v", err)
	}
}
