package usage

import "context"

type featureKey struct{}

// WithFeature 는 ctx 에 비용 기록용 feature 라벨을 심는다.
func WithFeature(ctx context.Context, feature string) context.Context {
	return context.WithValue(ctx, featureKey{}, feature)
}

// FeatureFromContext 는 ctx 의 feature 라벨을 꺼낸다(없으면 "").
func FeatureFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(featureKey{}).(string); ok {
		return v
	}
	return ""
}
