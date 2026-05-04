package agents

import "context"

// RunFunc is the core run function shape. Middleware wraps RunFuncs to inject
// cross-cutting behavior (retry, tracing, prompt augmentation, etc.) without
// modifying the Runner.
type RunFunc func(ctx context.Context, agent *Agent, opts RunOptions) (*RunResult, error)

// Middleware wraps a RunFunc to add behavior. Middlewares are applied in
// order: chain[0] sees the call first, chain[len-1] is closest to the
// underlying Runner.
type Middleware func(next RunFunc) RunFunc

// chain composes middlewares right-to-left so that chain[0] runs first.
func chain(base RunFunc, mws []Middleware) RunFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		base = mws[i](base)
	}
	return base
}
