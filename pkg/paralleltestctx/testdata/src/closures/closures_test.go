package closures

import (
	"context"
	"testing"
	"time"
)

func check(_ context.Context, _ string) error { return nil }

// Bug: the closure captures the parent's timeout context and a parallel
// subtest calls it. Mirrors https://github.com/coder/coder/pull/31011.
func TestClosureCapturesParentCtxWarn(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	as := func(user string) context.Context {
		_ = check(ctx, user)
		return ctx
	}
	_ = as("parent")

	for _, name := range []string{"a", "b"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_ = as(name) // want "timeout context ctx used after a t.Parallel call via closure as"
		})
	}
}

// Bug: closures that reference other closures are followed.
func TestTransitiveClosureWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	inner := func() error { return check(ctx, "x") }
	outer := func() error { return inner() }
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = outer() // want "timeout context ctx used after a t.Parallel call via closure outer"
	})
}

// Bug: a closure declared inside the subtest still captures the parent's
// context.
func TestClosureInsideSubtestCapturesParentCtxWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		load := func() error { return check(ctx, "x") }
		_ = load() // want "timeout context ctx used after a t.Parallel call via closure load"
	})
}

// Bug: the top-level test calls the closure after its own t.Parallel.
func TestClosureAfterTopLevelParallelWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	load := func() error { return check(ctx, "x") }
	t.Parallel()
	_ = load() // want "timeout context ctx used after a t.Parallel call via closure load"
}

// OK: the closure takes the context as a parameter, like the fix in
// https://github.com/coder/coder/pull/31011.
func TestClosureTakesCtxOK(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	as := func(ctx context.Context, user string) context.Context {
		_ = check(ctx, user)
		return ctx
	}
	_ = as(ctx, "parent")

	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = as(ctx, "sub")
	})
}

// OK: the subtest is not parallel, so it runs before the parent returns.
func TestClosureNonParallelSubtestOK(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	load := func() error { return check(ctx, "x") }
	t.Run("sub", func(t *testing.T) {
		_ = load()
	})
}

// OK: the closure is called before t.Parallel.
func TestClosureBeforeParallelOK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	load := func() error { return check(ctx, "x") }
	t.Run("sub", func(t *testing.T) {
		_ = load()
		t.Parallel()
	})
}

// OK: the closure captures a context created after t.Parallel.
func TestClosureCapturesSubtestCtxOK(t *testing.T) {
	t.Parallel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		load := func() error { return check(ctx, "x") }
		_ = load()
	})
}

// OK: the captured context is overwritten with a non-timeout context before
// the call.
func TestClosureCtxOverwrittenOK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	load := func() error { return check(ctx, "x") }
	ctx = context.Background()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = load()
	})
}

// OK: the closure captures only values that are safe to share.
func TestClosureCapturesPlainValuesOK(t *testing.T) {
	t.Parallel()
	prefix := "user-"
	name := func(s string) string { return prefix + s }
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = name("a")
	})
}
