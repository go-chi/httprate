package httprate

import "context"

type ctxKey int

const (
	incrementKey ctxKey = iota
	requestLimitKey
)

// WithIncrement sets the increment value in the context.
func WithIncrement(ctx context.Context, value int) context.Context {
	return context.WithValue(ctx, incrementKey, value)
}

// getIncrement gets the increment value from the context, which was set by
// [WithIncrement].
func getIncrement(ctx context.Context) int {
	if value, ok := ctx.Value(incrementKey).(int); ok {
		return value
	}
	return 1
}

// WithRequestLimit sets the request limit in the context.
func WithRequestLimit(ctx context.Context, value int) context.Context {
	return context.WithValue(ctx, requestLimitKey, value)
}

// getRequestLimit gets the request limit from the context, which was set by
// [WithRequestLimit].
func getRequestLimit(ctx context.Context) int {
	if value, ok := ctx.Value(requestLimitKey).(int); ok {
		return value
	}
	return 0
}
