package agent

import (
	"sync"

	"google.golang.org/genai"
)

// maxHistory 는 대화 채널당 보관할 최근 turn(content) 수다(user/model 교대라 짝수 권장).
const maxHistory = 16

// memory 는 채널별 최근 대화 맥락을 메모리에 보관한다(서버 재시작 시 초기화).
type memory struct {
	mu    sync.Mutex
	turns map[string][]*genai.Content
}

func newMemory() *memory {
	return &memory{turns: map[string][]*genai.Content{}}
}

// get 은 채널의 대화 맥락 복사본을 반환한다.
func (m *memory) get(channel string) []*genai.Content {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.turns[channel]
	out := make([]*genai.Content, len(src))
	copy(out, src)
	return out
}

// add 는 사용자 발화와 자비스 응답을 맥락에 추가하고 최근 maxHistory 개로 자른다.
func (m *memory) add(channel, userText, assistantText string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	turns := append(m.turns[channel],
		&genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{{Text: userText}}},
		&genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{{Text: assistantText}}},
	)
	if len(turns) > maxHistory {
		turns = turns[len(turns)-maxHistory:]
	}
	m.turns[channel] = turns
}
