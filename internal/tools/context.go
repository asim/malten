package tools

import "context"

// CallInfo carries per-request state into tool execution without widening the
// Tool interface. Right now it holds the request-scoped IssueBook that the
// issue tools read and mutate. The agent attaches it before executing a tool.
type CallInfo struct {
	Issues *IssueBook
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
