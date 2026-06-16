package agent

import (
	"context"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

// Tool 은 에이전트가 호출할 수 있는 도구다.
// 읽기 도구는 Run(즉시 실행), 쓰기 도구는 Propose(변경안 생성)만 채운다.
type Tool struct {
	Decl    *genai.FunctionDeclaration
	Write   bool
	Run     func(ctx context.Context, args map[string]any) (string, error)
	Propose func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error)
}

// strArg 는 args 에서 문자열 인자를 안전하게 꺼낸다(없으면 "").
func strArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// intArg 는 args 에서 정수 인자를 꺼낸다(없으면 nil). JSON 숫자는 float64 로 온다.
func intArg(args map[string]any, key string) *int {
	switch v := args[key].(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		return &v
	}
	return nil
}

// objSchema 는 object 파라미터 스키마를 만든다.
func objSchema(props map[string]*genai.Schema, required ...string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeObject, Properties: props, Required: required}
}

func strSchema(desc string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeString, Description: desc}
}
func intSchema(desc string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeInteger, Description: desc}
}

// toolDecls 는 도구 목록에서 genai 도구 선언을 만든다.
func toolDecls(tools []Tool) []*genai.Tool {
	decls := make([]*genai.FunctionDeclaration, len(tools))
	for i, t := range tools {
		decls[i] = t.Decl
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// toolMap 은 이름→Tool 맵을 만든다.
func toolMap(tools []Tool) map[string]Tool {
	m := make(map[string]Tool, len(tools))
	for _, t := range tools {
		m[t.Decl.Name] = t
	}
	return m
}
