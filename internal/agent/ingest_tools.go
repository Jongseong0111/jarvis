package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

// IngestTools 는 KB ingest 관련 도구 목록을 만든다.
func IngestTools(port IngestPort) []Tool {
	k := &ingestTools{port: port}
	return []Tool{k.startConceptIngest(), k.resumeKBReview()}
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
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				// kb 레포에서 브랜치를 생성한다
				branchCmd := exec.Command("git", "checkout", "-b", branch)
				branchCmd.Dir = k.port.KBPath
				if out, err := branchCmd.CombinedOutput(); err != nil {
					slog.Default().Error("브랜치 생성 실패", "branch", branch, "out", string(out), "error", err)
					_ = k.port.Sender.Send(bgCtx, domain.Reply{
						ChannelID: channelID,
						Text:      "🚨 브랜치 생성 중 문제가 생겼어요: " + string(out),
					})
					k.port.Registry.Exit(channelID)
					return
				}

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

func (k *ingestTools) resumeKBReview() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "resume_kb_review",
			Description: "서버 재시작 후 기존 KB ingest 세션을 리뷰 모드로 복원한다. session_id 를 모르면 생략해도 된다(최신 세션 자동 조회).",
			Parameters: objSchema(map[string]*genai.Schema{
				"session_id": strSchema("복원할 claude 세션 UUID. 모르면 빈 문자열로 전달하면 자동 조회"),
				"branch":     strSchema("ingest 브랜치명 (예: kb/ingest-rest-api). 모르면 빈 문자열"),
			}, "branch"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			channelID := channelIDFromCtx(ctx)
			if channelID == "" {
				return "", fmt.Errorf("채널 ID 없음")
			}
			if _, ok := k.port.Registry.Get(channelID); ok {
				return "이미 리뷰 세션이 활성화되어 있어요.", nil
			}
			sessionID := strArg(args, "session_id")
			if sessionID == "" {
				var err error
				sessionID, err = latestKBSessionID(k.port.KBPath)
				if err != nil {
					return "최신 KB 세션을 찾지 못했어요: " + err.Error(), nil
				}
			}
			branch := strArg(args, "branch")
			if branch == "" {
				branch = "(알 수 없음)"
			}
			k.port.Registry.Enter(channelID, ReviewSession{
				SessionID: sessionID,
				Branch:    branch,
				Busy:      false,
			})
			return fmt.Sprintf("✅ 리뷰 모드 복원 완료. 브랜치: `%s`\n이제 수정 요청을 보내거나 \"승인\"으로 PR을 만들 수 있어요.", branch), nil
		},
	}
}

// latestKBSessionID 는 kb 레포에 해당하는 claude 세션 중 가장 최근 것을 반환한다.
func latestKBSessionID(kbPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// claude 는 절대경로의 '/' 를 '-' 로 치환해 ~/.claude/projects/ 아래 저장한다.
	// 예) /Users/foo/kb → -Users-foo-kb
	dirName := strings.ReplaceAll(kbPath, "/", "-")
	sessionDir := filepath.Join(home, ".claude", "projects", dirName)

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return "", fmt.Errorf("세션 디렉터리 없음: %s", sessionDir)
	}
	var latest os.DirEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if latest == nil {
			latest = e
			continue
		}
		li, _ := latest.Info()
		ei, _ := e.Info()
		if ei != nil && li != nil && ei.ModTime().After(li.ModTime()) {
			latest = e
		}
	}
	if latest == nil {
		return "", fmt.Errorf("세션 파일 없음")
	}
	return strings.TrimSuffix(latest.Name(), ".jsonl"), nil
}

// slugFrom 은 소스 파일 경로에서 git 브랜치명 안전 slug 를 만든다.
// 공백·한글 등 비ASCII 는 하이픈으로 치환하고 중복 하이픈을 정리한다.
func slugFrom(sourcePath string) string {
	base := filepath.Base(sourcePath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	slug := b.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "ingest"
	}
	return slug
}
