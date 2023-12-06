package httprate

import "context"

var incrementKey = &struct{}{}

func WithIncrement(ctx context.Context, value int) context.Context {
	return context.WithValue(ctx, incrementKey, value)
}

func getIncrement(ctx context.Context) int {
	if value, ok := ctx.Value(incrementKey).(int); ok {
		return value
	}
	return 1
}
