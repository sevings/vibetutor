package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// FileStore stores each user's session as a JSON file inside a directory.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFile creates the directory (if needed) and returns a FileStore.
func NewFile(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", dir, err)
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) path(userID int64) string {
	return filepath.Join(s.dir, strconv.FormatInt(userID, 10)+".json")
}

// Load returns the stored session, or a fresh one when the user is new.
func (s *FileStore) Load(userID int64) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path(userID))
	if errors.Is(err, fs.ErrNotExist) {
		return &Session{Mode: ModeChat}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session %d: %w", userID, err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("decode session %d: %w", userID, err)
	}
	return &sess, nil
}

// Save atomically writes the session to disk.
func (s *FileStore) Save(userID int64, sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session %d: %w", userID, err)
	}

	tmp, err := os.CreateTemp(s.dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write session %d: %w", userID, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session %d: %w", userID, err)
	}
	if err := os.Rename(tmpName, s.path(userID)); err != nil {
		return fmt.Errorf("store session %d: %w", userID, err)
	}
	return nil
}
