// Package log 는 slog 기반 구조화 로거와 컨텍스트 전달을 제공한다.
package log

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey struct{}

// New 는 환경에 맞는 로거를 만든다. local 은 사람이 읽기 쉬운 text, 그 외는 JSON.
func New(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var handler slog.Handler
	if env == "local" || env == "" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// WithContext 는 로거를 컨텍스트에 싣는다.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext 는 컨텍스트의 로거를 반환한다. 없으면 기본 로거.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
