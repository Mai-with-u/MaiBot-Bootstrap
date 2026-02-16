package registry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const schemaVersion = 1

type Entry struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Path        string    `json:"path"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Index struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

type Store struct {
	path string
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (Index, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Index{Version: schemaVersion, Entries: []Entry{}}, nil
		}
		return Index{}, err
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, err
	}
	if idx.Version == 0 {
		idx.Version = schemaVersion
	}
	if idx.Entries == nil {
		idx.Entries = []Entry{}
	}
	return idx, nil
}

func (s *Store) Save(idx Index) error {
	idx.Version = schemaVersion
	if idx.Entries == nil {
		idx.Entries = []Entry{}
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *Store) Upsert(entry Entry) error {
	idx, err := s.Load()
	if err != nil {
		return err
	}

	entry.ID = strings.TrimSpace(entry.ID)
	entry.DisplayName = strings.TrimSpace(entry.DisplayName)
	entry.Path = strings.TrimSpace(entry.Path)
	if entry.ID == "" || entry.DisplayName == "" || entry.Path == "" {
		return errors.New("registry entry is incomplete")
	}

	replaced := false
	for i := range idx.Entries {
		if idx.Entries[i].ID == entry.ID {
			if entry.CreatedAt.IsZero() {
				entry.CreatedAt = idx.Entries[i].CreatedAt
			}
			idx.Entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Entries = append(idx.Entries, entry)
	}

	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].DisplayName < idx.Entries[j].DisplayName
	})
	return s.Save(idx)
}

func (s *Store) RemoveByID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}

	idx, err := s.Load()
	if err != nil {
		return err
	}

	out := make([]Entry, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	idx.Entries = out
	return s.Save(idx)
}

func (s *Store) Resolve(ref string) (Entry, bool, error) {
	idx, err := s.Load()
	if err != nil {
		return Entry{}, false, err
	}
	needle := strings.TrimSpace(ref)
	if needle == "" {
		return Entry{}, false, nil
	}

	for _, e := range idx.Entries {
		if e.ID == needle {
			return e, true, nil
		}
	}
	for _, e := range idx.Entries {
		if e.DisplayName == needle {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

func (s *Store) List() ([]Entry, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(idx.Entries))
	copy(entries, idx.Entries)
	return entries, nil
}
