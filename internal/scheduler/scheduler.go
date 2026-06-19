// Package scheduler 는 "매일 HH:MM 에 실행" 작업을 도는 인프로세스 스케줄러다.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jongseong0111/jarvis/pkg/log"
)

// Job 은 매일 지정 시각에 실행할 작업이다.
type Job struct {
	Name    string
	Hour    int
	Min     int
	TZ      *time.Location
	Timeout time.Duration // 0 이면 30s 기본
	Fn      func(ctx context.Context)
}

// Scheduler 는 등록된 Job 들을 각자 goroutine 으로 돌린다.
type Scheduler struct {
	jobs []Job
}

// New 는 빈 스케줄러를 만든다.
func New() *Scheduler { return &Scheduler{} }

// Register 는 Job 을 등록한다(Run 전에 호출).
func (s *Scheduler) Register(j Job) { s.jobs = append(s.jobs, j) }

// Run 은 모든 Job 을 goroutine 으로 시작하고 ctx 종료까지 블록한다.
func (s *Scheduler) Run(ctx context.Context) {
	for _, j := range s.jobs {
		go s.runJob(ctx, j)
	}
	<-ctx.Done()
}

func (s *Scheduler) runJob(ctx context.Context, j Job) {
	logger := log.FromContext(ctx)
	for {
		// fire 종료 후 현재 시각 기준으로 다음 발화를 재계산 — 실행이 예정 시각을 넘겨도 당일 재실행은 없음
		next := nextFire(time.Now().In(j.TZ), j.Hour, j.Min, j.TZ)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.fire(ctx, j, logger)
		}
	}
}

// fire 는 Job.Fn 을 recover + 타임아웃으로 1회 실행한다.
func (s *Scheduler) fire(ctx context.Context, j Job, logger *slog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("스케줄 작업 패닉", "job", j.Name, "recover", r)
		}
	}()
	timeout := j.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	j.Fn(runCtx)
}

// nextFire 는 now 이후 가장 가까운 hour:min 시각을 tz 기준으로 계산한다(정각 일치면 내일).
func nextFire(now time.Time, hour, min int, tz *time.Location) time.Time {
	now = now.In(tz)
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, tz)
	if !candidate.After(now) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate
}
