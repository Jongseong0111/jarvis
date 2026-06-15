package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/router"
	"github.com/Jongseong0111/jarvis/internal/slack"
	"github.com/Jongseong0111/jarvis/internal/workers"
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

	classifier := router.NewGeminiClassifier(cfg.GeminiAPIKey, cfg.GeminiModel)
	msgRouter := router.NewRouter(
		classifier,
		map[string]domain.Worker{
			"home":      workers.NewHome(),
			"knowledge": workers.NewKnowledge(),
		},
		workers.NewSystem(), // system.* 및 매핑 없는 intent fallback
	)
	handler := slack.NewHandler(msgRouter, client)

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
