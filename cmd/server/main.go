package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/gemini"
	"github.com/Jongseong0111/jarvis/internal/notion"
	"github.com/Jongseong0111/jarvis/internal/slack"
	"github.com/Jongseong0111/jarvis/pkg/config"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

func main() {
	logger := log.New(os.Getenv("JARVIS_ENV"))

	cfg, err := config.New()
	if err != nil {
		logger.Error("설정 로드 실패", "error", err)
		os.Exit(1)
	}

	client, err := slack.NewClient(cfg.SlackBotToken, cfg.SlackAppToken)
	if err != nil {
		logger.Error("slack 클라이언트 생성 실패", "error", err)
		os.Exit(1)
	}

	// 도구를 가진 LLM 에이전트(자연 대화 + 집정리 도구)
	geminiClient := gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel)
	home := agent.NewNotionHome(
		notion.New(cfg.NotionAPIKey),
		cfg.NotionLocationsDBID, cfg.NotionCategoriesDBID, cfg.NotionItemsDBID,
	)
	ag := agent.New(geminiClient, agent.HomeTools(home), "")
	handler := slack.NewHandler(ag, client)

	// 버튼 승인 처리(변경안 적용). applier=HomeApplier, sender=client
	client.SetInteractionHandler(slack.NewInteractionHandler(agent.NewHomeApplier(home), client))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.WithContext(ctx, logger)

	logger.Info("jarvis 시작", "env", cfg.Env)
	if err := client.Run(ctx, handler); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("정상 종료")
			return
		}
		logger.Error("실행 종료", "error", err)
		os.Exit(1)
	}
}
