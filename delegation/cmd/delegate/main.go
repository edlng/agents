// Command delegate is the Stage 5 runtime. It reaches models only through the
// Anthropic API provider.
//
//	delegate run --task delegation/fixtures/endpoint-allowlist [--budget 3]
//	delegate replay --task DIR --exchanges FILE   (no credentials needed)
//	delegate status <correlation-id>
//	delegate decide <correlation-id> --decision approve|reject|continue --reviewer NAME [--note TEXT]
//	delegate verify <correlation-id>
//	delegate smoke [--model M]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/dispatch"
	"github.com/edlng/agents/litmus-eval/delegation/internal/gate"
	"github.com/edlng/agents/litmus-eval/delegation/internal/harness"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider/anthropic"
	"github.com/edlng/agents/litmus-eval/delegation/internal/smoke"
)

const usage = `usage:
  delegate run --task DIR [--budget USD] [--runs DIR] [--coordinator-model M] [--worker-model M]
  delegate replay --task DIR --exchanges FILE [--runs DIR]
  delegate status ID [--runs DIR]
  delegate decide ID --decision approve|reject|continue --reviewer NAME [--note TEXT] [--key GPGKEY] [--runs DIR]
  delegate verify ID [--runs DIR]
  delegate smoke [--model M] [--runs DIR]`

func main() {
	if len(os.Args) < 2 {
		fail(fmt.Errorf("%s", usage))
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	runs := fs.String("runs", "delegation/runs", "run directory root")
	switch cmd {
	case "run":
		taskDir := fs.String("task", "", "task directory")
		budget := fs.Float64("budget", dispatch.DefaultLimits.BudgetUSD, "run budget in USD")
		coordModel := fs.String("coordinator-model", "", "override the coordinator model")
		workerModel := fs.String("worker-model", "", "override every sub-agent model")
		record := fs.String("record", "", "append every model exchange to this JSONL file")
		fs.Parse(args)
		requireKey()
		limits := dispatch.DefaultLimits
		limits.BudgetUSD = *budget
		var p provider.Provider = anthropic.New()
		if *record != "" {
			rec, err := provider.NewRecorder(p, *record)
			if err != nil {
				fail(err)
			}
			defer rec.Close()
			p = rec
		}
		res, err := harness.Run(context.Background(), harness.Config{
			Provider: p, ProviderName: "anthropic-api", TaskDir: *taskDir, RunsRoot: *runs, Limits: limits,
			CoordinatorModel: *coordModel, WorkerModel: *workerModel,
		})
		print(res)
		if err != nil {
			fail(err)
		}
	case "replay":
		taskDir := fs.String("task", "", "task directory")
		exchanges := fs.String("exchanges", "", "recorded exchanges JSONL")
		fs.Parse(args)
		rp, err := provider.LoadReplay(*exchanges)
		if err != nil {
			fail(err)
		}
		rp.Strict = false
		res, err := harness.Run(context.Background(), harness.Config{Provider: rp, ProviderName: "replay", TaskDir: *taskDir, RunsRoot: *runs})
		print(map[string]any{"result": res, "request_hash_mismatches": rp.Mismatches})
		if err != nil {
			fail(err)
		}
	case "status":
		id := positional(fs, args)
		status, err := gate.Status(filepath.Join(*runs, id))
		if err != nil {
			fail(err)
		}
		ds, _ := gate.Decisions(filepath.Join(*runs, id))
		print(map[string]any{"correlation_id": id, "status": status, "decisions": ds})
	case "decide":
		decision := fs.String("decision", "", "approve, reject, or continue")
		reviewer := fs.String("reviewer", "", "reviewer identity")
		note := fs.String("note", "", "optional note")
		key := fs.String("key", "", "gpg signing key (default: git user.signingkey, then user.email)")
		id := positional(fs, args)
		d, err := gate.Record(*runs, id, *reviewer, *decision, *note, gate.GPG{Key: *key})
		if err != nil {
			fail(err)
		}
		print(d)
	case "verify":
		id := positional(fs, args)
		s, err := audit.Verify(filepath.Join(*runs, id, "audit.jsonl"))
		if err != nil {
			fail(err)
		}
		ds, err := gate.VerifyDecisions(filepath.Join(*runs, id), gate.GPG{})
		if err != nil {
			fail(err)
		}
		status, err := gate.Status(filepath.Join(*runs, id))
		if err != nil {
			fail(err)
		}
		print(map[string]any{"audit": s, "status": status, "signed_decisions": len(ds)})
	case "smoke":
		model := fs.String("model", "claude-opus-5-5", "model ID")
		fs.Parse(args)
		requireKey()
		s, err := smoke.Run(context.Background(), anthropic.New(), *model, *runs)
		if err != nil {
			fail(err)
		}
		print(s)
	default:
		fail(fmt.Errorf("unknown command %q\n%s", cmd, usage))
	}
}

// positional takes the run ID before or after the flags.
func positional(fs *flag.FlagSet, args []string) string {
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		fs.Parse(args[1:])
		return args[0]
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fail(fmt.Errorf("expected one correlation ID\n%s", usage))
	}
	return fs.Arg(0)
}

func requireKey() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		fail(fmt.Errorf("ANTHROPIC_API_KEY is not set"))
	}
}

func print(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
