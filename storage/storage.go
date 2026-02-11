package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Message represents a single chat message.
type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Chat represents a conversation with metadata.
type Chat struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	CreatedAt int64     `json:"created_at"`
	UpdatedAt int64     `json:"updated_at"`
}

// Preferences stores user preferences.
type Preferences struct {
	SelectedModel string `json:"selected_model"`
}

// Store handles file-based persistence of chats and preferences.
type Store struct {
	dir string
	mu  sync.RWMutex
}

// New creates a new Store with the given data directory.
func New(dir string) (*Store, error) {
	chatsDir := filepath.Join(dir, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		return nil, fmt.Errorf("create chats directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// SaveChat persists a chat to disk.
func (s *Store) SaveChat(chat *Chat) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat.UpdatedAt = time.Now().Unix()
	data, err := json.MarshalIndent(chat, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chat: %w", err)
	}

	path := filepath.Join(s.dir, "chats", chat.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write chat file: %w", err)
	}
	return nil
}

// LoadChat reads a chat from disk by ID.
func (s *Store) LoadChat(id string) (*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Sanitize ID to prevent path traversal
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || id == ".." {
		return nil, fmt.Errorf("invalid chat ID")
	}

	path := filepath.Join(s.dir, "chats", id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read chat file: %w", err)
	}

	var chat Chat
	if err := json.Unmarshal(data, &chat); err != nil {
		return nil, fmt.Errorf("unmarshal chat: %w", err)
	}
	return &chat, nil
}

// ListChats returns all chats sorted by updated_at descending (newest first).
func (s *Store) ListChats() ([]Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chatsDir := filepath.Join(s.dir, "chats")
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		return nil, fmt.Errorf("read chats directory: %w", err)
	}

	var chats []Chat
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(chatsDir, entry.Name()))
		if err != nil {
			continue
		}

		var chat Chat
		if err := json.Unmarshal(data, &chat); err != nil {
			continue
		}
		// Don't include full messages in the list — just metadata
		chat.Messages = nil
		chats = append(chats, chat)
	}

	sort.Slice(chats, func(i, j int) bool {
		return chats[i].UpdatedAt > chats[j].UpdatedAt
	})

	return chats, nil
}

// DeleteChat removes a chat from disk.
func (s *Store) DeleteChat(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.Contains(id, "/") || strings.Contains(id, "\\") || id == ".." {
		return fmt.Errorf("invalid chat ID")
	}

	path := filepath.Join(s.dir, "chats", id+".json")
	return os.Remove(path)
}

// LoadPreferences reads user preferences from disk.
func (s *Store) LoadPreferences() (*Preferences, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "preferences.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Preferences{}, nil
		}
		return nil, fmt.Errorf("read preferences: %w", err)
	}

	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, fmt.Errorf("unmarshal preferences: %w", err)
	}
	return &prefs, nil
}

// SavePreferences persists user preferences to disk.
func (s *Store) SavePreferences(prefs *Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}

	path := filepath.Join(s.dir, "preferences.json")
	return os.WriteFile(path, data, 0644)
}
