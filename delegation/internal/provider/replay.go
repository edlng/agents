package provider

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Exchange is one recorded request/response pair.
type Exchange struct {
	RequestSHA256 string   `json:"request_sha256"`
	Response      Response `json:"response"`
}

// RequestHash identifies a request so a replay can detect prompt drift.
func RequestHash(req Request) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Recorder wraps a live provider and appends every exchange to a JSONL file.
type Recorder struct {
	Inner Provider
	mu    sync.Mutex
	f     *os.File
}

func NewRecorder(inner Provider, path string) (*Recorder, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Recorder{Inner: inner, f: f}, nil
}

func (r *Recorder) Complete(ctx context.Context, req Request) (Response, error) {
	resp, err := r.Inner.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	hash, err := RequestHash(req)
	if err != nil {
		return resp, err
	}
	line, err := json.Marshal(Exchange{RequestSHA256: hash, Response: resp})
	if err != nil {
		return resp, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.f.Write(append(line, '\n')); err != nil {
		return resp, fmt.Errorf("record exchange: %w", err)
	}
	return resp, r.f.Sync()
}

func (r *Recorder) Close() error { return r.f.Close() }

// Replay serves recorded responses in order. In strict mode a request whose
// hash differs from the recording fails, so a changed prompt cannot silently
// reuse old output. Non-strict mode counts mismatches instead; use it for
// recordings whose tool results vary between runs (test timings).
type Replay struct {
	Strict     bool
	Mismatches int
	mu         sync.Mutex
	exchanges  []Exchange
	next       int
}

func NewReplay(exchanges []Exchange) *Replay { return &Replay{exchanges: exchanges, Strict: true} }

func LoadReplay(path string) (*Replay, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var exchanges []Exchange
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for line := 1; scanner.Scan(); line++ {
		var e Exchange
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		exchanges = append(exchanges, e)
	}
	return NewReplay(exchanges), scanner.Err()
}

func (r *Replay) Complete(_ context.Context, req Request) (Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.next >= len(r.exchanges) {
		return Response{}, fmt.Errorf("replay exhausted after %d exchanges", len(r.exchanges))
	}
	e := r.exchanges[r.next]
	if e.RequestSHA256 != "" {
		hash, err := RequestHash(req)
		if err != nil {
			return Response{}, err
		}
		if hash != e.RequestSHA256 && !r.Strict {
			r.Mismatches++
		} else if hash != e.RequestSHA256 {
			return Response{}, fmt.Errorf("replay exchange %d: request hash %s does not match recording %s", r.next, hash, e.RequestSHA256)
		}
	}
	r.next++
	return e.Response, nil
}
