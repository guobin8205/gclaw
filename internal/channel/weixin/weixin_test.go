package weixin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractText(t *testing.T) {
	tests := []struct {
		name  string
		items []MessageItem
		want  string
	}{
		{
			name:  "empty items",
			items: nil,
			want:  "",
		},
		{
			name: "single text item",
			items: []MessageItem{
				{Type: ItemTypeText, TextItem: &TextItem{Text: "hello"}},
			},
			want: "hello",
		},
		{
			name: "multiple text items concatenated",
			items: []MessageItem{
				{Type: ItemTypeText, TextItem: &TextItem{Text: "hello "}},
				{Type: ItemTypeText, TextItem: &TextItem{Text: "world"}},
			},
			want: "hello world",
		},
		{
			name: "voice item with transcription",
			items: []MessageItem{
				{Type: ItemTypeVoice, VoiceItem: &VoiceItem{Text: "语音消息内容"}},
			},
			want: "语音消息内容",
		},
		{
			name: "mixed text and image (skip image)",
			items: []MessageItem{
				{Type: ItemTypeText, TextItem: &TextItem{Text: "看这张图"}},
				{Type: ItemTypeImage, ImageItem: &ImageItem{URL: "http://example.com/img.jpg"}},
			},
			want: "看这张图",
		},
		{
			name: "nil text item",
			items: []MessageItem{
				{Type: ItemTypeText, TextItem: nil},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractText(tt.items)
			if got != tt.want {
				t.Errorf("extractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStore(t *testing.T) {
	// Use temp home to avoid polluting real config.
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", tmpHome)
	defer os.Setenv("USERPROFILE", origUserProfile)

	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// List should be empty initially.
	ids, err := s.ListAccountIDs()
	if err != nil {
		t.Fatalf("ListAccountIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(ids))
	}

	// Register an account.
	if err := s.RegisterAccount("test-bot"); err != nil {
		t.Fatalf("RegisterAccount: %v", err)
	}

	ids, _ = s.ListAccountIDs()
	if len(ids) != 1 || ids[0] != "test-bot" {
		t.Errorf("expected [test-bot], got %v", ids)
	}

	// Duplicate register should be no-op.
	if err := s.RegisterAccount("test-bot"); err != nil {
		t.Errorf("duplicate RegisterAccount: %v", err)
	}
	ids, _ = s.ListAccountIDs()
	if len(ids) != 1 {
		t.Errorf("expected still 1 account, got %d", len(ids))
	}

	// Save credentials.
	acc := &AccountData{
		Token:   "test-token-123",
		BaseURL: "https://ilinkai.weixin.qq.com",
		UserID:  "user@im.wechat",
		SavedAt: "2026-04-26T12:00:00Z",
	}
	if err := s.SaveAccount("test-bot", acc); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}

	// Load credentials.
	loaded, err := s.LoadAccount("test-bot")
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	if loaded.Token != "test-token-123" {
		t.Errorf("token = %q, want %q", loaded.Token, "test-token-123")
	}
	if loaded.BaseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", loaded.BaseURL, DefaultBaseURL)
	}

	// Load non-existent account returns nil, no error.
	nonexistent, err := s.LoadAccount("not-found")
	if err != nil {
		t.Errorf("LoadAccount(not-found): %v", err)
	}
	if nonexistent != nil {
		t.Errorf("expected nil for non-existent account")
	}

	// Delete account.
	if err := s.DeleteAccount("test-bot"); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	ids, _ = s.ListAccountIDs()
	if len(ids) != 0 {
		t.Errorf("expected 0 accounts after delete, got %d", len(ids))
	}
}

func TestStoreDataDir(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpHome)
	os.Setenv("USERPROFILE", tmpHome)
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("USERPROFILE", origUserProfile)

	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	expected := filepath.Join(tmpHome, ".gclaw", "weixin")
	if s.DataDir() != expected {
		t.Errorf("DataDir = %q, want %q", s.DataDir(), expected)
	}
}

func TestChannelStatus(t *testing.T) {
	ch := &Channel{
		cfg:       Config{Verbose: false},
		connected: true,
		accountID: "test@im.bot",
		userID:    "user@im.wechat",
		msgCount:  5,
	}

	s := ch.Status()
	if !s.Connected {
		t.Error("expected connected")
	}
	if s.AccountID != "test@im.bot" {
		t.Errorf("accountID = %q", s.AccountID)
	}
	if s.MsgCount != 5 {
		t.Errorf("msgCount = %d", s.MsgCount)
	}
}

func TestChannelInit(t *testing.T) {
	ch, err := New(Config{Verbose: false})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ch.ID() != "weixin" {
		t.Errorf("ID = %q, want %q", ch.ID(), "weixin")
	}
	if ch.IsConnected() {
		t.Error("new channel should not be connected")
	}
}

func TestMessageItemTypes(t *testing.T) {
	// Verify type constants match the protocol specification.
	if ItemTypeText != 1 {
		t.Error("ItemTypeText should be 1")
	}
	if ItemTypeImage != 2 {
		t.Error("ItemTypeImage should be 2")
	}
	if ItemTypeVoice != 3 {
		t.Error("ItemTypeVoice should be 3")
	}
	if ItemTypeFile != 4 {
		t.Error("ItemTypeFile should be 4")
	}
	if ItemTypeVideo != 5 {
		t.Error("ItemTypeVideo should be 5")
	}
}
