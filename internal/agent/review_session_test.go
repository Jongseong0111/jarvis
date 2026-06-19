package agent_test

import (
	"testing"

	"github.com/Jongseong0111/jarvis/internal/agent"
)

func TestReviewSessionRegistry_enterExit(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()

	_, ok := r.Get("ch1")
	if ok {
		t.Fatal("없는 채널인데 found")
	}

	r.Enter("ch1", agent.ReviewSession{Branch: "kb/ingest-foo", Slug: "foo"})
	s, ok := r.Get("ch1")
	if !ok {
		t.Fatal("Enter 후 Get 실패")
	}
	if s.Branch != "kb/ingest-foo" {
		t.Errorf("branch: got %q", s.Branch)
	}

	r.Exit("ch1")
	_, ok = r.Get("ch1")
	if ok {
		t.Fatal("Exit 후 여전히 존재")
	}
}

func TestReviewSessionRegistry_busy(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()
	r.Enter("ch1", agent.ReviewSession{Slug: "foo"})

	r.SetBusy("ch1", true)
	s, _ := r.Get("ch1")
	if !s.Busy {
		t.Fatal("SetBusy(true) 후 Busy=false")
	}

	r.SetBusy("ch1", false)
	s, _ = r.Get("ch1")
	if s.Busy {
		t.Fatal("SetBusy(false) 후 Busy=true")
	}
}

func TestReviewSessionRegistry_setSessionID(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()
	r.Enter("ch1", agent.ReviewSession{Slug: "foo"})
	r.SetSessionID("ch1", "ses_abc")
	s, _ := r.Get("ch1")
	if s.SessionID != "ses_abc" {
		t.Errorf("session_id: got %q", s.SessionID)
	}
}

func TestReviewSessionRegistry_exitNoop(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()
	r.Exit("nonexistent") // should not panic
}
