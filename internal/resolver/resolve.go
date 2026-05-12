package resolver

import "context"

// Resolve is the general resolver entrypoint for v1.
//
// It currently dispatches to the existing ResolveNPM implementation,
// which already handles npm/git/path/workspace source kinds.
func Resolve(ctx context.Context, opts ResolverOptions, req ResolveRequest) ResolveResult {
	return ResolveNPM(ctx, opts, req)
}
