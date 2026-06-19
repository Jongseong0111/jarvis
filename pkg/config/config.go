// Package config 는 .env/환경변수에서 jarvis 설정을 로드하고 검증한다.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config 는 jarvis 실행에 필요한 설정값이다.
type Config struct {
	Env               string
	SlackBotToken     string
	SlackAppToken     string
	GeminiAPIKey      string
	GeminiModel       string
	GeminiVisionModel string
	KnowledgeRepoPath string

	NotionAPIKey         string
	NotionLocationsDBID  string
	NotionCategoriesDBID string
	NotionItemsDBID      string
	NotionHomeURL        string // 집 정리 페이지 링크(선택, 사용자에게 보여줄 용도)
	NotionMapPageID      string // 자동 렌더링할 "우리집 지도" 페이지 ID(선택)

	TodoistAPIToken        string
	TodoistBriefingChannel string
	TodoistMorning         string // "HH:MM"
	TodoistEvening         string // "HH:MM"
	TodoistTZ              string

	DigestTime    string   // "HH:MM", 기본 "09:00"
	DigestRSSURLs []string // 추가 RSS 피드 URL 목록
}

// New 는 config/.env(있으면)와 환경변수에서 설정을 로드하고 필수값을 검증한다.
func New() (Config, error) {
	_ = godotenv.Load("config/.env") // 파일 없으면 무시 (환경변수만으로도 동작)

	cfg := Config{
		Env:               getenv("JARVIS_ENV", "local"),
		SlackBotToken:     os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:     os.Getenv("SLACK_APP_TOKEN"),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiModel:       getenv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiVisionModel: getenv("GEMINI_VISION_MODEL", "gemini-2.5-flash-lite"),
		KnowledgeRepoPath: expandHome(getenv("KNOWLEDGE_REPO_PATH", "~/personal-agent/knowledge-base")),

		NotionAPIKey:         os.Getenv("NOTION_API_KEY"),
		NotionLocationsDBID:  os.Getenv("NOTION_LOCATIONS_DB_ID"),
		NotionCategoriesDBID: os.Getenv("NOTION_CATEGORIES_DB_ID"),
		NotionItemsDBID:      os.Getenv("NOTION_ITEMS_DB_ID"),
		NotionHomeURL:        os.Getenv("NOTION_HOME_URL"),
		NotionMapPageID:      os.Getenv("NOTION_MAP_PAGE_ID"),

		TodoistAPIToken:        os.Getenv("TODOIST_API_TOKEN"),
		TodoistBriefingChannel: os.Getenv("TODOIST_BRIEFING_CHANNEL"),
		TodoistMorning:         getenv("TODOIST_MORNING_TIME", "08:00"),
		TodoistEvening:         getenv("TODOIST_EVENING_TIME", "21:00"),
		TodoistTZ:              getenv("TODOIST_BRIEFING_TZ", "Asia/Seoul"),

		DigestTime:    getenv("DIGEST_TIME", "09:00"),
		DigestRSSURLs: parseCommaList(os.Getenv("DIGEST_RSS_URLS")),
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

// expandHome 은 "~/" 접두를 사용자 홈 디렉터리로 치환한다.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// parseCommaList 는 쉼표로 구분된 문자열을 슬라이스로 파싱한다(빈 값 제거).
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ParseHHMM 은 "HH:MM" 문자열을 시·분으로 파싱한다.
func ParseHHMM(s string) (hour, min int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("시각 형식이 HH:MM 이 아님: %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("시(hour) 가 잘못됨: %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("분(min) 이 잘못됨: %q", s)
	}
	return h, m, nil
}
