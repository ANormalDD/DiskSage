package cleaner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"disksage/internal/models"
)

type HistoryStore struct {
	path string
	mu   sync.Mutex
}

type HistoryRecord struct {
	Timestamp time.Time           `json:"timestamp"`
	Requester string              `json:"requester"`
	Summary   models.CleanSummary `json:"summary"`
}

func NewHistoryStore(path string) *HistoryStore {
	if path == "" {
		path = filepath.Join(os.TempDir(), "disksage", "history.jsonl")
	}
	return &HistoryStore{path: path}
}

func (h *HistoryStore) Append(record HistoryRecord) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	blob, _ := json.Marshal(record)
	_, err = f.Write(append(blob, '\n'))
	return err
}
