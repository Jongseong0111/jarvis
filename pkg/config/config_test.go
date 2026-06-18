package config

import (
	"os"
	"testing"
)

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
	// 필수 env 채우고 VISION 모델은 비워 기본값 확인
	env := map[string]string{
		"SLACK_BOT_TOKEN": "x", "SLACK_APP_TOKEN": "x", "GEMINI_API_KEY": "x",
		"NOTION_API_KEY": "x", "NOTION_LOCATIONS_DB_ID": "x",
		"NOTION_CATEGORIES_DB_ID": "x", "NOTION_ITEMS_DB_ID": "x",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
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
	env := map[string]string{
		"SLACK_BOT_TOKEN": "x", "SLACK_APP_TOKEN": "x", "GEMINI_API_KEY": "x",
		"NOTION_API_KEY": "x", "NOTION_LOCATIONS_DB_ID": "x",
		"NOTION_CATEGORIES_DB_ID": "x", "NOTION_ITEMS_DB_ID": "x",
		"GEMINI_VISION_MODEL": "gemini-3.1-flash-lite",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.GeminiVisionModel != "gemini-3.1-flash-lite" {
		t.Fatalf("오버라이드 = %q", cfg.GeminiVisionModel)
	}
}
