// Package audit persists a hash-chained JSONL trail per examination. Every
// event is fsynced before Append returns, so the trail survives a crash and is
// available for post-hoc review without the process that wrote it.
package audit

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Event kinds.
const (
	KindLLMCall    = "llm_call"
	KindToolCall   = "tool_call"
	KindToolResult = "tool_result"
	KindDispatch   = "dispatch"
	KindValidation = "validation"
	KindGate       = "gate"
	KindDecision   = "decision"
	KindTerminal   = "terminal"
)

type Event struct {
	Seq           int             `json:"seq"`
	Time          string          `json:"time"`
	CorrelationID string          `json:"correlation_id"`
	SpanID        string          `json:"span_id"`
	ParentSpanID  string          `json:"parent_span_id,omitempty"`
	Kind          string          `json:"kind"`
	Agent         string          `json:"agent,omitempty"`
	Workflow      string          `json:"workflow,omitempty"`
	Step          string          `json:"step,omitempty"`
	Model         string          `json:"model,omitempty"`
	InputTokens   int64           `json:"input_tokens,omitempty"`
	OutputTokens  int64           `json:"output_tokens,omitempty"`
	CacheRead     int64           `json:"cache_read_tokens,omitempty"`
	CacheWrite    int64           `json:"cache_write_tokens,omitempty"`
	CostUSD       float64         `json:"cost_usd,omitempty"`
	DurationMS    int64           `json:"duration_ms,omitempty"`
	ResponseID    string          `json:"response_id,omitempty"`
	Detail        json.RawMessage `json:"detail,omitempty"`
	PrevHash      string          `json:"prev_hash"`
	Hash          string          `json:"hash"`
}

func hashEvent(e Event) (string, error) {
	e.Hash = ""
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// NewID returns a sortable random identifier such as 20261005T153012Z-9f2c1a7b.
func NewID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// Log appends events for one correlation ID.
type Log struct {
	mu            sync.Mutex
	f             *os.File
	correlationID string
	dir           string
	seq           int
	prev          string
}

// Create starts a new trail at root/<correlationID>/audit.jsonl. It refuses to
// overwrite an existing trail.
func Create(root, correlationID string) (*Log, error) {
	dir := filepath.Join(root, correlationID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create audit trail: %w", err)
	}
	return &Log{f: f, correlationID: correlationID, dir: dir}, nil
}

// Resume reopens an existing trail to append to it, continuing its hash chain.
func Resume(root, correlationID string) (*Log, error) {
	dir := filepath.Join(root, correlationID)
	s, err := Verify(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("resume audit trail: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Log{f: f, correlationID: correlationID, dir: dir, seq: s.Events, prev: s.HeadHash}, nil
}

func (l *Log) CorrelationID() string { return l.correlationID }

// Dir is the run directory holding the trail and its artifacts.
func (l *Log) Dir() string { return l.dir }

// Append fills Seq, Time, CorrelationID, and the hash chain, then writes and
// fsyncs the event. It returns the event as written.
func (l *Log) Append(e Event) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	e.Seq = l.seq
	e.CorrelationID = l.correlationID
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.SpanID == "" {
		e.SpanID = fmt.Sprintf("s%04d", e.Seq)
	}
	e.PrevHash = l.prev
	hash, err := hashEvent(e)
	if err != nil {
		return e, err
	}
	e.Hash = hash
	line, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return e, fmt.Errorf("write audit event %d: %w", e.Seq, err)
	}
	if err := l.f.Sync(); err != nil {
		return e, fmt.Errorf("sync audit event %d: %w", e.Seq, err)
	}
	l.prev = hash
	return e, nil
}

// Close writes summary.json next to the trail and closes the file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.f.Close(); err != nil {
		return err
	}
	s, err := Verify(filepath.Join(l.dir, "audit.jsonl"))
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.dir, "summary.json"), append(b, '\n'), 0o644)
}

type Totals struct {
	Calls        int     `json:"calls"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheRead    int64   `json:"cache_read_tokens"`
	CacheWrite   int64   `json:"cache_write_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int64   `json:"duration_ms"`
}

func (t *Totals) add(e Event) {
	t.Calls++
	t.InputTokens += e.InputTokens
	t.OutputTokens += e.OutputTokens
	t.CacheRead += e.CacheRead
	t.CacheWrite += e.CacheWrite
	t.CostUSD += e.CostUSD
	t.DurationMS += e.DurationMS
}

type Summary struct {
	CorrelationID string            `json:"correlation_id"`
	Events        int               `json:"events"`
	HeadHash      string            `json:"head_hash"`
	Total         Totals            `json:"total"`
	ByAgent       map[string]Totals `json:"by_agent"`
	Models        []string          `json:"models"`
}

// Verify re-reads a trail from disk, checks the hash chain, sequence, and
// correlation ID of every event, and totals the LLM calls.
func Verify(path string) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, err
	}
	defer f.Close()
	s := Summary{ByAgent: map[string]Totals{}}
	models := map[string]bool{}
	prev := ""
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return s, fmt.Errorf("event %d: %w", s.Events+1, err)
		}
		s.Events++
		if e.Seq != s.Events {
			return s, fmt.Errorf("event %d: seq is %d", s.Events, e.Seq)
		}
		if s.CorrelationID == "" {
			s.CorrelationID = e.CorrelationID
		}
		if e.CorrelationID != s.CorrelationID {
			return s, fmt.Errorf("event %d: correlation ID %s, trail is %s", e.Seq, e.CorrelationID, s.CorrelationID)
		}
		if e.PrevHash != prev {
			return s, fmt.Errorf("event %d: prev_hash does not match event %d", e.Seq, e.Seq-1)
		}
		want, err := hashEvent(e)
		if err != nil {
			return s, err
		}
		if e.Hash != want {
			return s, fmt.Errorf("event %d: hash mismatch, event was modified", e.Seq)
		}
		prev = e.Hash
		if e.Kind == KindLLMCall {
			s.Total.add(e)
			t := s.ByAgent[e.Agent]
			t.add(e)
			s.ByAgent[e.Agent] = t
			models[e.Model] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return s, err
	}
	s.HeadHash = prev
	for m := range models {
		s.Models = append(s.Models, m)
	}
	sort.Strings(s.Models)
	return s, nil
}
