// Package config 는 .env/환경변수에서 jarvis 설정을 로드하고 검증한다.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config 는 jarvis 실행에 필요한 설정값이다.
type Config struct {
	Env           string
	SlackBotToken string
	SlackAppToken string
	GeminiAPIKey  string
	GeminiModel   string

	NotionAPIKey         string
	NotionLocationsDBID  string
	NotionCategoriesDBID string
	NotionItemsDBID      string
	NotionHomeURL        string // 집 정리 페이지 링크(선택, 사용자에게 보여줄 용도)
	NotionMapPageID      string // 자동 렌더링할 "우리집 지도" 페이지 ID(선택)
}

// New 는 config/.env(있으면)와 환경변수에서 설정을 로드하고 필수값을 검증한다.
func New() (Config, error) {
	_ = godotenv.Load("config/.env") // 파일 없으면 무시 (환경변수만으로도 동작)

	cfg := Config{
		Env:           getenv("JARVIS_ENV", "local"),
		SlackBotToken: os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken: os.Getenv("SLACK_APP_TOKEN"),
		GeminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		GeminiModel:   getenv("GEMINI_MODEL", "gemini-2.5-flash-lite"),

		NotionAPIKey:         os.Getenv("NOTION_API_KEY"),
		NotionLocationsDBID:  os.Getenv("NOTION_LOCATIONS_DB_ID"),
		NotionCategoriesDBID: os.Getenv("NOTION_CATEGORIES_DB_ID"),
		NotionItemsDBID:      os.Getenv("NOTION_ITEMS_DB_ID"),
		NotionHomeURL:        os.Getenv("NOTION_HOME_URL"),
		NotionMapPageID:      os.Getenv("NOTION_MAP_PAGE_ID"),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.SlackBotToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN 이 비어있습니다")
	}
	if c.SlackAppToken == "" {
		return fmt.Errorf("SLACK_APP_TOKEN 이 비어있습니다")
	}
	if c.GeminiAPIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY 가 비어있습니다")
	}
	if c.NotionAPIKey == "" {
		return fmt.Errorf("NOTION_API_KEY 가 비어있습니다")
	}
	if c.NotionLocationsDBID == "" || c.NotionCategoriesDBID == "" || c.NotionItemsDBID == "" {
		return fmt.Errorf("NOTION_{LOCATIONS,CATEGORIES,ITEMS}_DB_ID 가 비어있습니다")
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
