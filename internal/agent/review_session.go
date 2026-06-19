package agent

import "sync"

// ReviewSession 은 채널당 활성 Claude Code 리뷰 세션이다.
type ReviewSession struct {
	SessionID  string
	Branch     string
	SourcePath string
	Slug       string
	Busy       bool
}

// ReviewSessionRegistry 는 채널별 리뷰 세션 상태를 스레드 안전하게 관리한다.
type ReviewSessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]*ReviewSession
}

// NewReviewSessionRegistry 는 빈 레지스트리를 만든다.
func NewReviewSessionRegistry() *ReviewSessionRegistry {
	return &ReviewSessionRegistry{sessions: make(map[string]*ReviewSession)}
}

// Enter 는 채널을 리뷰 모드로 진입시킨다(기존 세션을 덮어씀).
func (r *ReviewSessionRegistry) Enter(channelID string, s ReviewSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := s
	r.sessions[channelID] = &cp
}

// Exit 는 채널의 리뷰 모드를 종료한다.
func (r *ReviewSessionRegistry) Exit(channelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, channelID)
}

// Get 은 채널의 세션 스냅샷을 반환한다. 없으면 ok=false.
func (r *ReviewSessionRegistry) Get(channelID string) (ReviewSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[channelID]
	if !ok {
		return ReviewSession{}, false
	}
	return *s, true
}

// SetBusy 는 채널 세션의 busy 상태를 갱신한다(세션 없으면 no-op).
func (r *ReviewSessionRegistry) SetBusy(channelID string, busy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[channelID]; ok {
		s.Busy = busy
	}
}

// SetSessionID 는 비동기 Run 완료 후 session_id 를 저장한다(세션 없으면 no-op).
func (r *ReviewSessionRegistry) SetSessionID(channelID, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[channelID]; ok {
		s.SessionID = sessionID
	}
}
