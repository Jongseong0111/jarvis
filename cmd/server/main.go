package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/gemini"
	"github.com/Jongseong0111/jarvis/internal/knowledge"
	"github.com/Jongseong0111/jarvis/internal/notion"
	"github.com/Jongseong0111/jarvis/internal/scheduler"
	"github.com/Jongseong0111/jarvis/internal/slack"
	"github.com/Jongseong0111/jarvis/internal/todoist"
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

	// ctx 는 config 로드 직후에 생성한다(스케줄러 등이 ctx 를 필요로 하므로).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.WithContext(ctx, logger)

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

	knowledgeSvc := knowledge.NewService(geminiClient, cfg.KnowledgeRepoPath)
	tools := append(agent.HomeTools(home, cfg.NotionHomeURL, mapURL), agent.KnowledgeTools(knowledgeSvc)...)

	// 변경안 적용기: 기본은 집정리, delete_todo 는 Todoist 로 분기
	var applier domain.ProposalApplier = agent.NewHomeApplier(home, renderer)

	if cfg.TodoistAPIToken != "" {
		todoistClient := todoist.New(cfg.TodoistAPIToken)
		tools = append(tools, agent.TodoistTools(todoistClient)...)
		applier = agent.NewDispatchApplier(
			map[string]domain.ProposalApplier{"delete_todo": agent.NewTodoistApplier(todoistClient)},
			applier,
		)
		if err := startBriefings(ctx, cfg, todoistClient, client, logger); err != nil {
			logger.Error("브리핑 스케줄러 시작 실패", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("Todoist 비활성(TODOIST_API_TOKEN 없음)")
	}

	ag := agent.New(geminiClient, visionClient, tools, "")
	handler := slack.NewHandler(ag, client)

	// 버튼 승인 처리(변경안 적용 + 지도 갱신). applier=applier(Todoist 활성 시 DispatchApplier, 아니면 HomeApplier), sender=client
	client.SetInteractionHandler(slack.NewInteractionHandler(applier, client))

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

// startBriefings 는 아침/저녁 브리핑을 스케줄러에 등록하고 백그라운드로 돌린다.
func startBriefings(ctx context.Context, cfg config.Config, client agent.TodoistPort, sender domain.MessageSender, logger *slog.Logger) error {
	if cfg.TodoistBriefingChannel == "" {
		logger.Info("브리핑 채널 없음 — 스케줄러 미기동(도구만 활성)")
		return nil
	}
	tz, err := time.LoadLocation(cfg.TodoistTZ)
	if err != nil {
		return fmt.Errorf("타임존 로드(%s): %w", cfg.TodoistTZ, err)
	}
	mh, mm, err := config.ParseHHMM(cfg.TodoistMorning)
	if err != nil {
		return fmt.Errorf("아침 시각: %w", err)
	}
	eh, em, err := config.ParseHHMM(cfg.TodoistEvening)
	if err != nil {
		return fmt.Errorf("저녁 시각: %w", err)
	}
	sched := scheduler.New()
	sched.Register(scheduler.Job{Name: "morning", Hour: mh, Min: mm, TZ: tz,
		Fn: agent.NewMorningBriefing(client, sender, cfg.TodoistBriefingChannel)})
	sched.Register(scheduler.Job{Name: "evening", Hour: eh, Min: em, TZ: tz,
		Fn: agent.NewEveningBriefing(client, sender, cfg.TodoistBriefingChannel)})
	go sched.Run(ctx)
	logger.Info("브리핑 스케줄러 기동", "morning", cfg.TodoistMorning, "evening", cfg.TodoistEvening, "tz", cfg.TodoistTZ)
	return nil
}
