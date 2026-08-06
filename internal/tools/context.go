package tools

import "context"

// CallInfo carries per-request context (which session the tool is acting on)
// into tool execution without widening the Tool interface. The agent attaches
// it before executing a tool; tools that need it read it back.
type CallInfo struct {
	SessionID string
}

type ctxKey struct{}

// WithCallInfo returns a context carrying the call info.
func WithCallInfo(ctx context.Context, ci CallInfo) context.Context {
	return context.WithValue(ctx, ctxKey{}, ci)
}

// CallInfoFrom extracts call info from the context (zero value if absent).
func CallInfoFrom(ctx context.Context) CallInfo {
	ci, _ := ctx.Value(ctxKey{}).(CallInfo)
	return ci
}
