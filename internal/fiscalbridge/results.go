package fiscalbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ResultStore remembers, on the cash desk PC, the outcomes the local driver
// already produced, keyed by the idempotency key of the job. A job
// delivered again (the ack was lost, the lease expired, the app restarted)
// is acknowledged with the remembered result instead of printing a second
// receipt: together with the idempotent Submit on the instance this gives
// exactly-once issuing as long as the store survives.
type ResultStore interface {
	Get(key string) (Result, bool)
	Put(key string, r Result) error
}

// MaxRememberedResults bounds a ResultStore (oldest entries are dropped);
// a job is redelivered within minutes, so a few thousand is ample.
const MaxRememberedResults = 2000

type remembered struct {
	Result Result    `json:"result"`
	At     time.Time `json:"at"`
}

// MemoryResults is an in-process ResultStore (tests, or no data dir).
type MemoryResults struct {
	mu sync.Mutex
	m  map[string]remembered
}

// NewMemoryResults returns an empty MemoryResults.
func NewMemoryResults() *MemoryResults { return &MemoryResults{m: map[string]remembered{}} }

// Get implements ResultStore.
func (s *MemoryResults) Get(key string) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.m[key]
	return r.Result, ok
}

// Put implements ResultStore.
func (s *MemoryResults) Put(key string, r Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = remembered{Result: r, At: time.Now().UTC()}
	prune(s.m)
	return nil
}

func prune(m map[string]remembered) {
	if len(m) <= MaxRememberedResults {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]].At.Before(m[keys[j]].At) })
	for _, k := range keys[:len(m)-MaxRememberedResults] {
		delete(m, k)
	}
}

// FileResults is a ResultStore persisted as JSON in the data directory
// (0600, atomic rename), so outcomes survive an app restart.
type FileResults struct {
	path string
	mem  *MemoryResults
}

// OpenFileResults loads (or creates) the store at path. A corrupted file is
// moved aside and the store starts empty.
func OpenFileResults(path string) (*FileResults, error) {
	f := &FileResults{path: path, mem: NewMemoryResults()}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return f, nil
	case err != nil:
		return nil, fmt.Errorf("fiscalbridge: read results: %w", err)
	}
	if err := json.Unmarshal(b, &f.mem.m); err != nil {
		_ = os.Rename(path, path+".corrupted")
		f.mem.m = map[string]remembered{}
	}
	return f, nil
}

// Get implements ResultStore.
func (f *FileResults) Get(key string) (Result, bool) { return f.mem.Get(key) }

// Put implements ResultStore (written through before returning, so the
// outcome is on disk before it is submitted).
func (f *FileResults) Put(key string, r Result) error {
	f.mem.mu.Lock()
	defer f.mem.mu.Unlock()
	f.mem.m[key] = remembered{Result: r, At: time.Now().UTC()}
	prune(f.mem.m)
	b, err := json.Marshal(f.mem.m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("fiscalbridge: write results: %w", err)
	}
	return os.Rename(tmp, f.path)
}
