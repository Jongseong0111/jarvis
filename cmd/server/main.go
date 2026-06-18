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
	visionClient := gemini.New(cfg.GeminiAPIKey, cfg.GeminiVisionModel)
	notionClient := notion.New(cfg.NotionAPIKey)
	home := agent.NewNotionHome(
		notionClient,
		cfg.NotionLocationsDBID, cfg.NotionCategoriesDBID, cfg.NotionItemsDBID,
	)

	// 우리집 지도 자동 렌더러(맵 페이지가 설정된 경우)
	mapURL := ""
	var renderer *agent.MapRenderer
	if cfg.NotionMapPageID != "" {
		renderer = agent.NewMapRenderer(notionClient, cfg.NotionMapPageID, home)
		mapURL = "https://www.notion.so/" + cfg.NotionMapPageID
	}

	ag := agent.New(geminiClient, visionClient, agent.HomeTools(home, cfg.NotionHomeURL, mapURL), "")
	handler := slack.NewHandler(ag, client)

	// 버튼 승인 처리(변경안 적용 + 지도 갱신). applier=HomeApplier, sender=client
	client.SetInteractionHandler(slack.NewInteractionHandler(agent.NewHomeApplier(home, renderer), client))

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
