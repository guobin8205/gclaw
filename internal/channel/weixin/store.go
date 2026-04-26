package weixin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store persists weixin account credentials.
type Store struct {
	dir string
}

// NewStore creates a new weixin credential store.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".gclaw", "weixin")
	return &Store{dir: dir}, nil
}

func (s *Store) accountsDir() string {
	return filepath.Join(s.dir, "accounts")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.dir, "accounts.json")
}

func (s *Store) accountPath(id string) string {
	return filepath.Join(s.accountsDir(), id+".json")
}

// ensureDirs creates the store directories if needed.
func (s *Store) ensureDirs() error {
	if err := os.MkdirAll(s.accountsDir(), 0700); err != nil {
		return err
	}
	return nil
}

// ListAccountIDs returns all registered account IDs.
func (s *Store) ListAccountIDs() ([]string, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// RegisterAccount adds an account ID to the index.
func (s *Store) RegisterAccount(id string) error {
	ids, _ := s.ListAccountIDs()
	for _, existing := range ids {
		if existing == id {
			return nil
		}
	}
	ids = append(ids, id)
	if err := s.ensureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0600)
}

// UnregisterAccount removes an account ID from the index.
func (s *Store) UnregisterAccount(id string) error {
	ids, _ := s.ListAccountIDs()
	filtered := make([]string, 0, len(ids))
	for _, existing := range ids {
		if existing != id {
			filtered = append(filtered, existing)
		}
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0600)
}

// LoadAccount loads stored credentials for an account.
func (s *Store) LoadAccount(id string) (*AccountData, error) {
	data, err := os.ReadFile(s.accountPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var acc AccountData
	if err := json.Unmarshal(data, &acc); err != nil {
		return nil, err
	}
	return &acc, nil
}

// SaveAccount persists account credentials.
func (s *Store) SaveAccount(id string, acc *AccountData) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.accountPath(id), data, 0600)
}

// DeleteAccount removes all stored data for an account.
func (s *Store) DeleteAccount(id string) error {
	_ = os.Remove(s.accountPath(id))
	return s.UnregisterAccount(id)
}

// SaveChatUserID persists just the last chatting user ID for an account.
func (s *Store) SaveChatUserID(id, chatUserID string) error {
	acc, err := s.LoadAccount(id)
	if err != nil || acc == nil {
		return err
	}
	acc.ChatUserID = chatUserID
	return s.SaveAccount(id, acc)
}

// DataDir returns the store directory for display.
func (s *Store) DataDir() string {
	return s.dir
}
