// Package router 는 수신 메시지를 intent 로 분류하고 해당 Worker 로 디스패치한다.
package router

import (
	"context"
	"fmt"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// Router 는 Classifier 로 intent 를 판단하고 namespace 기준으로 Worker 를 선택한다.
// domain.MessageRouter 를 구현한다.
type Router struct {
	classifier domain.Classifier
	workers    map[string]domain.Worker // key = intent namespace ("home"/"knowledge")
	fallback   domain.Worker            // 매핑 없는 namespace(system 포함) 처리
}

// NewRouter 는 Router 를 생성한다. fallback 은 매핑되지 않은 intent(주로 system.*)를 처리한다.
func NewRouter(classifier domain.Classifier, workers map[string]domain.Worker, fallback domain.Worker) Router {
	return Router{classifier: classifier, workers: workers, fallback: fallback}
}

// Route 는 메시지를 분류해 해당 Worker 에 위임한다.
func (r Router) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	intent, err := r.classifier.Classify(ctx, in.Text)
	if err != nil {
		return domain.Reply{}, fmt.Errorf("intent 분류 실패: %w", err)
	}
	log.FromContext(ctx).Info("intent 분류", "intent", string(intent), "text", in.Text)

	worker, ok := r.workers[intent.Namespace()]
	if !ok {
		worker = r.fallback
	}
	return worker.Handle(ctx, intent, in)
}
