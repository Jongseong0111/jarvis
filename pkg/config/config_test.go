package config

import "testing"

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
