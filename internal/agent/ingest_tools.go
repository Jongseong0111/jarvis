package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// IngestPort 는 start_concept_ingest 도구가 필요로 하는 의존성이다.
type IngestPort struct {
	Runner   claudecode.Runner
	Registry *ReviewSessionRegistry
	Sender   domain.MessageSender
	KBPath   string
}

type ingestTools struct {
	port IngestPort
}

// IngestTools 는 start_concept_ingest 도구 목록을 만든다.
func IngestTools(port IngestPort) []Tool {
	k := &ingestTools{port: port}
	return []Tool{k.startConceptIngest()}
}

func (k *ingestTools) startConceptIngest() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "start_concept_ingest",
			Description: "저장된 소스 파일을 개념 문서로 정리하는 Claude Code 세션을 시작한다. 완료 결과(개념 트리)는 이 채널에 게시된다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"source_path": strSchema("정리할 소스 파일 경로(knowledge-base 기준 상대경로, 예: sources/conversation/go-notes.md)"),
			}, "source_path"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			channelID := channelIDFromCtx(ctx)
			if channelID == "" {
				return "", fmt.Errorf("채널 ID 없음")
			}
			sourcePath := strArg(args, "source_path")
			slug := slugFrom(sourcePath)
			branch := "kb/ingest-" + slug

			// 이미 리뷰 모드면 중복 시작 방지
			if _, ok := k.port.Registry.Get(channelID); ok {
				return "이미 개념 정리 세션이 진행 중이에요. 완료 후 다시 시도해주세요.", nil
			}

			k.port.Registry.Enter(channelID, ReviewSession{
				Branch:     branch,
				SourcePath: sourcePath,
				Slug:       slug,
				Busy:       true,
			})

			prompt := fmt.Sprintf(
				"/kb-ingest %s --type=conversation\n끝나면 제안 개념 트리 + 각 개념 1줄 요약을 슬랙용으로 정리해서 보여줘. (이모지 분류, 드롭 항목 포함)",
				sourcePath,
			)

			go func() {
				bgCtx := context.Background()
				result, err := k.port.Runner.Run(bgCtx, k.port.KBPath, prompt)
				if err != nil {
					slog.Default().Error("ingest 실패", "channel", channelID, "error", err)
					_ = k.port.Sender.Send(bgCtx, domain.Reply{
						ChannelID: channelID,
						Text:      "🚨 개념 정리 중 문제가 생겼어요. 다시 시도해볼까요?",
					})
					k.port.Registry.Exit(channelID)
					return
				}
				k.port.Registry.SetSessionID(channelID, result.SessionID)
				k.port.Registry.SetBusy(channelID, false)
				_ = k.port.Sender.Send(bgCtx, domain.Reply{
					ChannelID: channelID,
					Text:      result.Text,
				})
			}()

			return "🧠 개념 정리를 시작했습니다. 잠시 기다려주세요...", nil
		},
	}
}

// slugFrom 은 소스 파일 경로에서 브랜치/slug 이름을 만든다.
func slugFrom(sourcePath string) string {
	base := filepath.Base(sourcePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
