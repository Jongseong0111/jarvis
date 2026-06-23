package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

type todoistTools struct {
	port TodoistPort
}

// TodoistTools 는 할일 도구 목록을 만든다.
// add/list/complete/update 는 즉시 실행(Run), delete 만 변경안(Propose).
func TodoistTools(port TodoistPort) []Tool {
	t := todoistTools{port: port}
	return []Tool{t.listTodos(), t.addTodo(), t.completeTodo(), t.updateTodo(), t.deleteTodo()}
}

func (t todoistTools) listTodos() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name: "list_todos",
			Description: "할일을 조회한다. 사용자 의도에 맞는 filter 를 골라 넣어라:\n" +
				"- 미지정/오늘 할일: 비워두면 'today | overdue'(오늘+밀린).\n" +
				"- 전체(반복·스케줄 포함): filter=\"all\".\n" +
				"- 관리함/아직 일정 안 정한/마감 없는 할일: filter=\"no date\".\n" +
				"- 그 외 기간: Todoist 필터 문법(today, overdue, tomorrow, '7 days'(이번주), 'next week').",
			Parameters: objSchema(map[string]*genai.Schema{
				"filter": strSchema("Todoist 필터(선택). 기본 'today | overdue', 전체 'all', 마감없음 'no date', 이번주 '7 days'."),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			tasks, err := t.port.ListTasks(ctx, resolveFilter(strArg(args, "filter")))
			if err != nil {
				return "", err
			}
			if len(tasks) == 0 {
				return "할 일이 없습니다. 🎉", nil
			}
			return formatTaskLines(tasks), nil
		},
	}
}

// resolveFilter 는 사용자/LLM 이 준 필터 표현을 Todoist 필터 문법으로 정규화한다.
// 빈 값은 오늘+밀린, 전체 별칭은 빈 필터(/tasks), 관리함·마감없음 별칭은 "no date".
// 그 외는 Todoist 필터 문법으로 보고 그대로 통과시킨다.
func resolveFilter(input string) string {
	f := strings.TrimSpace(input)
	switch strings.ToLower(f) {
	case "":
		return "today | overdue"
	case "all", "전체", "모든", "모두":
		return "" // 빈 필터 → 전체 활성 할일(/tasks)
	case "inbox", "관리함", "인박스", "마감없음", "날짜없음", "기한없음", "일정없음", "no date", "no due date":
		return "no date"
	default:
		return f
	}
}

func (t todoistTools) addTodo() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "add_todo",
			Description: "할일을 추가한다. due 는 자연어 마감(예: '오늘', '내일 오후 3시', '매주 월요일')도 가능.",
			Parameters: objSchema(map[string]*genai.Schema{
				"content": strSchema("할일 내용. 예: Clone Graph 다시 풀기"),
				"due":     strSchema("마감(선택). 자연어 가능"),
				"project": strSchema("프로젝트 ID(선택)"),
			}, "content"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			content := strings.TrimSpace(strArg(args, "content"))
			if content == "" {
				return "", fmt.Errorf("할 일 내용을 알려주세요.")
			}
			task, err := t.port.AddTask(ctx, content, strArg(args, "due"), strArg(args, "project"))
			if err != nil {
				return "", err
			}
			if task.Due != "" {
				return fmt.Sprintf("✅ '%s' 추가했습니다. (마감: %s)", task.Content, task.Due), nil
			}
			return "✅ '" + task.Content + "' 추가했습니다.", nil
		},
	}
}

func (t todoistTools) completeTodo() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "complete_todo",
			Description: "할일을 완료 처리한다. query 로 내용을 찾는다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("완료할 할일 내용(부분일치). 예: Clone Graph"),
			}, "query"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			task, err := t.resolveTask(ctx, strArg(args, "query"))
			if err != nil {
				return "", err
			}
			if err := t.port.CompleteTask(ctx, task.ID); err != nil {
				return "", err
			}
			return "☑️ '" + task.Content + "' 완료 처리했습니다.", nil
		},
	}
}

func (t todoistTools) updateTodo() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "update_todo",
			Description: "할일의 내용 또는 마감을 수정한다. query 로 대상을 찾는다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query":   strSchema("수정할 할일 내용(부분일치)"),
				"content": strSchema("새 내용(선택)"),
				"due":     strSchema("새 마감(선택, 자연어 가능)"),
			}, "query"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			content := strings.TrimSpace(strArg(args, "content"))
			due := strings.TrimSpace(strArg(args, "due"))
			if content == "" && due == "" {
				return "", fmt.Errorf("무엇을 바꿀지 알려주세요(내용 또는 마감).")
			}
			task, err := t.resolveTask(ctx, strArg(args, "query"))
			if err != nil {
				return "", err
			}
			if err := t.port.UpdateTask(ctx, task.ID, content, due); err != nil {
				return "", err
			}
			if content != "" {
				return "✏️ '" + content + "'(으)로 수정했습니다.", nil
			}
			return "✏️ '" + task.Content + "' 마감을 '" + due + "'(으)로 변경했습니다.", nil
		},
	}
}

func (t todoistTools) deleteTodo() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "delete_todo",
			Description: "할일을 삭제한다. query 로 대상을 찾고 승인 버튼을 거친다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("삭제할 할일 내용(부분일치)"),
			}, "query"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			task, err := t.resolveTask(ctx, strArg(args, "query"))
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			return domain.ChangeProposal{
				Op:      "delete_todo",
				Summary: "🗑️ 다음 할 일을 삭제할까요?\n• " + task.Content,
				Fields:  map[string]string{"task_id": task.ID, "content": task.Content},
			}, nil
		},
	}
}

// resolveTask 는 query 로 미완료 할일 1개를 찾는다. 0개/다수면 에러(되묻기).
func (t todoistTools) resolveTask(ctx context.Context, query string) (todoist.Task, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return todoist.Task{}, fmt.Errorf("어떤 할 일인지 알려주세요.")
	}
	tasks, err := t.port.ListTasks(ctx, "")
	if err != nil {
		return todoist.Task{}, err
	}
	var matches []todoist.Task
	for _, tk := range tasks {
		if strings.Contains(strings.ToLower(tk.Content), strings.ToLower(query)) {
			matches = append(matches, tk)
		}
	}
	switch len(matches) {
	case 0:
		return todoist.Task{}, fmt.Errorf("'%s'에 해당하는 할 일을 찾지 못했습니다.", query)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.Content)
		}
		return todoist.Task{}, fmt.Errorf("'%s'에 해당하는 할 일이 여러 개예요: %s. 더 정확히 알려주세요.", query, strings.Join(names, ", "))
	}
}

// formatTaskLines 는 할일을 "• 내용 — 마감" 줄로 만든다.
func formatTaskLines(tasks []todoist.Task) string {
	var lines []string
	for _, tk := range tasks {
		line := "• " + tk.Content
		if tk.Due != "" {
			line += " — " + tk.Due
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
