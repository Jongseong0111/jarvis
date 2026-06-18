package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setRequiredEnv 은 모든 필수 환경변수를 설정한다.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	env := map[string]string{
		"SLACK_BOT_TOKEN": "x", "SLACK_APP_TOKEN": "x", "GEMINI_API_KEY": "x",
		"NOTION_API_KEY": "x", "NOTION_LOCATIONS_DB_ID": "x",
		"NOTION_CATEGORIES_DB_ID": "x", "NOTION_ITEMS_DB_ID": "x",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// full 은 모든 필수 값이 채워진 Config 를 반환한다.
func full() Config {
	return Config{
		SlackBotToken: "xoxb-1", SlackAppToken: "xapp-1", GeminiAPIKey: "AI-1",
		NotionAPIKey:        "ntn-1",
		NotionLocationsDBID: "loc-db", NotionCategoriesDBID: "cat-db", NotionItemsDBID: "item-db",
	}
}

// without 은 full() 에서 한 필드를 비운 Config 를 반환한다.
func without(mut func(*Config)) Config {
	c := full()
	mut(&c)
	return c
}

func TestConfig_validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "정상", cfg: full(), wantErr: false},
		{name: "bot 토큰 누락", cfg: without(func(c *Config) { c.SlackBotToken = "" }), wantErr: true},
		{name: "app 토큰 누락", cfg: without(func(c *Config) { c.SlackAppToken = "" }), wantErr: true},
		{name: "gemini 키 누락", cfg: without(func(c *Config) { c.GeminiAPIKey = "" }), wantErr: true},
		{name: "notion 키 누락", cfg: without(func(c *Config) { c.NotionAPIKey = "" }), wantErr: true},
		{name: "notion DB ID 누락", cfg: without(func(c *Config) { c.NotionItemsDBID = "" }), wantErr: true},
		{name: "모두 빈 값", cfg: Config{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNew_visionModelDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("GEMINI_VISION_MODEL")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.GeminiVisionModel != "gemini-2.5-flash-lite" {
		t.Fatalf("기본 비전 모델 = %q, want gemini-2.5-flash-lite", cfg.GeminiVisionModel)
	}
}

func TestNew_visionModelOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GEMINI_VISION_MODEL", "gemini-3.1-flash-lite")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.GeminiVisionModel != "gemini-3.1-flash-lite" {
		t.Fatalf("오버라이드 = %q", cfg.GeminiVisionModel)
	}
}

func TestNew_knowledgeRepoPathDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("KNOWLEDGE_REPO_PATH")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "personal-agent", "knowledge-base")
	if cfg.KnowledgeRepoPath != want {
		t.Fatalf("기본 경로 = %q, want %q", cfg.KnowledgeRepoPath, want)
	}
}

func TestNew_knowledgeRepoPathOverrideExpandsTilde(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_REPO_PATH", "~/kb")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.KnowledgeRepoPath != filepath.Join(home, "kb") {
		t.Fatalf("~ 확장 실패: %q", cfg.KnowledgeRepoPath)
	}
}
