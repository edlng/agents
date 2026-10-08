// Package gate holds the two human checkpoints: the substance gate, which
// elevates a review when its workflows are toy or mostly peripheral, and the
// append-only decision log that is the only way a run leaves PENDING_HUMAN or
// ELEVATED. Nothing the coordinator can call writes a decision.
package gate

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/catalog"
)

const (
	PendingHuman    = "PENDING_HUMAN"
	Elevated        = "ELEVATED"
	Complete        = "COMPLETE"
	RejectedHuman   = "REJECTED_HUMAN"
	FailedAutomated = "FAILED_AUTOMATED"
)

type SubstanceRow struct {
	Workflow  string `json:"workflow"`
	Substance string `json:"substance"`
	Rationale string `json:"rationale"`
}

type Substance struct {
	Rows     []SubstanceRow `json:"rows"`
	Elevated bool           `json:"elevated"`
	Reasons  []string       `json:"reasons,omitempty"`
}

// Assess applies the rule: any toy workflow, or two or more peripheral
// workflows, elevates the review for a human.
func Assess(c *catalog.Catalog) Substance {
	var s Substance
	// Every workflow is assessed, including the reviewer and the writer.
	ws := c.Client()
	for _, id := range []string{"adversarial-review", "final-review"} {
		if w := c.Workflows[id]; w != nil {
			ws = append(ws, w)
		}
	}
	peripheral := 0
	for _, w := range ws {
		s.Rows = append(s.Rows, SubstanceRow{Workflow: w.ID, Substance: w.Substance, Rationale: w.SubstanceRationale})
		switch w.Substance {
		case "toy":
			s.Reasons = append(s.Reasons, fmt.Sprintf("%s is scored toy", w.ID))
		case "peripheral":
			peripheral++
		}
	}
	if peripheral >= 2 {
		s.Reasons = append(s.Reasons, fmt.Sprintf("%d workflows are scored peripheral", peripheral))
	}
	s.Elevated = len(s.Reasons) > 0
	return s
}

// RunRecord is the system's record of a finished run. It is written once and
// never edited; decisions are appended beside it.
type RunRecord struct {
	CorrelationID string    `json:"correlation_id"`
	TaskID        string    `json:"task_id"`
	Provider      string    `json:"provider"`
	Workflows     string    `json:"workflows"` // catalog directory the run loaded
	Status        string    `json:"status"`    // PENDING_HUMAN | ELEVATED | FAILED_AUTOMATED
	Substance     Substance `json:"substance"`
	ReportSHA256  string    `json:"report_sha256,omitempty"`
	Requests      []string  `json:"human_requests,omitempty"`
	Coordinator   any       `json:"coordinator"`
	CostUSD       float64   `json:"cost_usd"`
}

type Decision struct {
	Time          string `json:"time"`
	CorrelationID string `json:"correlation_id"`
	Reviewer      string `json:"reviewer"`
	RecordedBy    string `json:"recorded_by"` // OS account that ran the command
	Decision      string `json:"decision"`
	Note          string `json:"note,omitempty"`
	FromStatus    string `json:"from_status"`
	ToStatus      string `json:"to_status"`
	ReportSHA256  string `json:"report_sha256"`
	// Signer is the fingerprint of the key that signed this record, and
	// Signature is a detached signature over the record with both fields
	// empty.
	Signer    string `json:"signer"`
	Signature string `json:"signature"`
}

// Signer signs and verifies decision records, so a decision is attributable
// to a key rather than to a typed name.
type Signer interface {
	Sign(data []byte) (signature, fingerprint string, err error)
	Verify(data []byte, signature string) (fingerprint string, err error)
}

func signedBytes(d Decision) ([]byte, error) {
	d.Signer, d.Signature = "", ""
	return json.Marshal(d)
}

// VerifyDecisions checks every decision signature in a run directory.
func VerifyDecisions(dir string, s Signer) ([]Decision, error) {
	ds, err := Decisions(dir)
	if err != nil {
		return nil, err
	}
	for i, d := range ds {
		data, err := signedBytes(d)
		if err != nil {
			return ds, err
		}
		fpr, err := s.Verify(data, d.Signature)
		if err != nil {
			return ds, fmt.Errorf("decision %d: %w", i+1, err)
		}
		if fpr != d.Signer {
			return ds, fmt.Errorf("decision %d: signed by %s, record names %s", i+1, fpr, d.Signer)
		}
	}
	return ds, nil
}

func ReadRun(dir string) (RunRecord, error) {
	var r RunRecord
	data, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(data, &r)
}

func Decisions(dir string) ([]Decision, error) {
	f, err := os.Open(filepath.Join(dir, "decisions.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Decision
	s := bufio.NewScanner(f)
	for s.Scan() {
		var d Decision
		if err := json.Unmarshal(s.Bytes(), &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, s.Err()
}

// Status is the run status after every recorded decision.
func Status(dir string) (string, error) {
	run, err := ReadRun(dir)
	if err != nil {
		return "", err
	}
	ds, err := Decisions(dir)
	if err != nil {
		return "", err
	}
	if len(ds) > 0 {
		return ds[len(ds)-1].ToStatus, nil
	}
	return run.Status, nil
}

var transitions = map[string]map[string]string{
	PendingHuman: {"approve": Complete, "reject": RejectedHuman},
	Elevated:     {"continue": PendingHuman, "reject": RejectedHuman},
}

// Record appends a human decision. It verifies the report on disk still
// matches the hash in run.json, so a decision always binds the report the
// system produced.
func Record(runsRoot, correlationID, reviewer, decision, note string, signer Signer) (Decision, error) {
	dir := filepath.Join(runsRoot, correlationID)
	if reviewer == "" {
		return Decision{}, fmt.Errorf("a reviewer identity is required")
	}
	if signer == nil {
		return Decision{}, fmt.Errorf("decisions must be signed")
	}
	run, err := ReadRun(dir)
	if err != nil {
		return Decision{}, err
	}
	from, err := Status(dir)
	if err != nil {
		return Decision{}, err
	}
	to, ok := transitions[from][decision]
	if !ok {
		return Decision{}, fmt.Errorf("status %s does not accept %q", from, decision)
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		return Decision{}, err
	}
	sum := sha256.Sum256(report)
	if hex.EncodeToString(sum[:]) != run.ReportSHA256 {
		return Decision{}, fmt.Errorf("report.md does not match the hash in run.json; refusing to record a decision")
	}
	d := Decision{
		Time: time.Now().UTC().Format(time.RFC3339), CorrelationID: correlationID,
		Reviewer: reviewer, RecordedBy: os.Getenv("USER"), Decision: decision, Note: note,
		FromStatus: from, ToStatus: to, ReportSHA256: run.ReportSHA256,
	}
	unsigned, err := signedBytes(d)
	if err != nil {
		return d, err
	}
	if d.Signature, d.Signer, err = signer.Sign(unsigned); err != nil {
		return d, fmt.Errorf("sign decision: %w", err)
	}
	line, err := json.Marshal(d)
	if err != nil {
		return d, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "decisions.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return d, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return d, err
	}
	if err := f.Close(); err != nil {
		return d, err
	}
	log, err := audit.Resume(runsRoot, correlationID)
	if err != nil {
		return d, err
	}
	if _, err := log.Append(audit.Event{Kind: audit.KindDecision, Agent: "human:" + reviewer, Detail: line}); err != nil {
		return d, err
	}
	return d, log.Close()
}
