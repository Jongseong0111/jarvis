// sendbrief 는 브리핑을 즉시 전송하는 일회성 CLI 도구다.
// 사용: go run ./cmd/sendbrief -kind=morning|evening|digest
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/devdigest"
	"github.com/Jongseong0111/jarvis/internal/gemini"
	islack "github.com/Jongseong0111/jarvis/internal/slack"
	"github.com/Jongseong0111/jarvis/internal/todoist"
	"github.com/Jongseong0111/jarvis/pkg/config"
	pkglog "github.com/Jongseong0111/jarvis/pkg/log"
)

func main() {
	kind := flag.String("kind", "morning", "브리핑 종류: morning | evening | digest")
	flag.Parse()

	logger := pkglog.New(os.Getenv("JARVIS_ENV"))

	cfg, err := config.New()
	if err != nil {
		logger.Error("설정 로드 실패", "error", err)
		os.Exit(1)
	}

	slackClient, err := islack.NewClient(cfg.SlackBotToken, cfg.SlackAppToken)
	if err != nil {
		logger.Error("slack 클라이언트 생성 실패", "error", err)
		os.Exit(1)
	}

	geminiClient := gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel)

	ch := cfg.TodoistBriefingChannel
	ctx := context.Background()

	logger.Info("브리핑 전송 중", "kind", *kind)

	switch *kind {
	case "morning":
		todoistClient := todoist.New(cfg.TodoistAPIToken)
		agent.NewMorningBriefing(todoistClient, nil, slackClient, ch)(ctx)
	case "evening":
		todoistClient := todoist.New(cfg.TodoistAPIToken)
		agent.NewEveningBriefing(todoistClient, slackClient, ch)(ctx)
	case "digest":
		fetcher := devdigest.NewFetcher(cfg.DigestRSSURLs)
		generator := devdigest.NewGenerator(geminiClient)
		agent.NewDevDigestBriefing(fetcher, generator, slackClient, ch)(ctx)
	default:
		logger.Error("알 수 없는 kind", slog.String("kind", *kind))
		os.Exit(1)
	}

	logger.Info("전송 완료", "kind", *kind)
}
