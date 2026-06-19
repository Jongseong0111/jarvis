package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// fakeBaseRouter 는 기록하는 MessageRouter 가짜 구현이다.
type fakeBaseRouter struct {
	called int
	reply  domain.Reply
}

func (f *fakeBaseRouter) Route(_ context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	f.called++
	f.reply.ChannelID = in.ChannelID
	f.reply.Text = "base:" + in.Text
	return f.reply, nil
}

func TestReviewRouter_notInReview_delegatesToBase(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{}
	sender := &fakeSender{}

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "안녕"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.Text != "base:안녕" {
		t.Errorf("text: %q", reply.Text)
	}
	if base.called != 1 {
		t.Errorf("base not called")
	}
}

func TestReviewRouter_inReview_busy_returnsBusyMessage(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Busy: true, Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "channel 빼"})
	if err != nil {
		t.Fatal(err)
	}
	if base.called != 0 {
		t.Error("busy 중엔 base 호출 안 해야 함")
	}
	if reply.Text == "" {
		t.Error("busy 안내 메시지 없음")
	}
}

func TestReviewRouter_inReview_cancel_exitsMode(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses1", Text: "ok"}}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Branch: "kb/ingest-foo", Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "취소"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text == "" {
		t.Error("취소 안내 없음")
	}
	_, still := registry.Get("ch1")
	if still {
		t.Error("취소 후 리뷰 모드 해제 안 됨")
	}
}

func TestReviewRouter_inReview_curate_sendsAsyncAndReturnsPending(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses1", Text: "수정됨"}}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "channel 빼"})
	if err != nil {
		t.Fatal(err)
	}
	// 동기 응답은 "처리 중" 안내여야 함
	if reply.ChannelID != "ch1" {
		t.Errorf("channel: %q", reply.ChannelID)
	}
	if reply.Text == "" {
		t.Error("처리 중 안내 없음")
	}

	// 고루틴이 slack 에 결과 게시할 때까지 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sender.mu.Lock()
		n := len(sender.replies)
		sender.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.replies) == 0 {
		t.Fatal("고루틴 slack 전송 없음")
	}
	if sender.replies[0].Text != "수정됨" {
		t.Errorf("async text: %q", sender.replies[0].Text)
	}
}

func TestReviewRouter_inReview_approve_exitsAndSendsAsync(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses1", Text: "PR 생성됨"}}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Branch: "kb/ingest-foo", Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	_, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "승인"})
	if err != nil {
		t.Fatal(err)
	}

	// 고루틴 완료 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sender.mu.Lock()
		n := len(sender.replies)
		sender.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 승인 후 리뷰 모드 해제
	_, still := registry.Get("ch1")
	if still {
		t.Error("승인 후 리뷰 모드 해제 안 됨")
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.replies) == 0 {
		t.Fatal("PR 결과 전송 없음")
	}
}
