package basic

import (
	"context"
	"testing"
	"time"
)

func TestInlineWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestFunctionUseWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		doThing(ctx) // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestInlineOK_Shadow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = ctx // ensure parent ctx is used in outer scope
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background() // shadow parent
		_ = ctx
	})
}

func TestFunctionUseOK_Shadow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = ctx // ensure parent ctx is used in outer scope
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background() // shadow parent
		doThing(ctx)
	})
}

func TestDeadlineWarn(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestNestedTimeoutWarn(t *testing.T) {
	ctx, cancel := wrapContext(context.WithTimeout(context.Background(), time.Second))
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestNestedTimeoutNonContextLHSOK(t *testing.T) {
	ok := contextWasCreated(context.WithTimeout(context.Background(), time.Second))
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ok
	})
}

func TestSamePackageTimeoutHelperWarn(t *testing.T) {
	ctx := helperTimeoutContext(t)
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestSamePackageTimeoutHelperChainWarn(t *testing.T) {
	ctx := chainedHelperTimeoutContext(t)
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		doThing(ctx) // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestNoParallelOK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(*testing.T) {
		_ = ctx
	})
}

func TestNonTimeoutOK(t *testing.T) {
	ctx := context.Background()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx
	})
}

func TestContextOverwrittenWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		ctx = context.Background() // want "timeout context ctx overwritten after a t.Parallel call; did you mean to shadow the variable\\?"
		_ = ctx
	})
}

// Case: Should warn - context created before t.Parallel, used after
func TestShouldWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second) // created before
	defer cancel()
	t.Parallel() // parallel call
	_ = ctx      // want "timeout context ctx used after a t.Parallel call"
}

// Case: Should NOT warn - context created after t.Parallel
func TestShouldNotWarn(t *testing.T) {
	t.Parallel()                                                          // parallel call
	ctx, cancel := context.WithTimeout(context.Background(), time.Second) // created after
	defer cancel()
	_ = ctx // should not warn
}

// Case: Should NOT warn - t.Parallel in subtest, context used in main test (different scopes)
func TestDifferentScopes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second) // created before
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel() // parallel call in subtest
	})
	_ = ctx // used in main test - should not warn (different scope)
}

func TestOverwriteEarly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	_ = ctx
	ctx = context.Background()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx
	})
}

func wrapContext(ctx context.Context, cancel context.CancelFunc) (context.Context, context.CancelFunc) {
	return ctx, cancel
}

func contextWasCreated(ctx context.Context, cancel context.CancelFunc) bool {
	cancel()
	return ctx != nil
}

func helperTimeoutContext(t *testing.T) context.Context {
	return wrapOnlyContext(context.WithTimeout(context.Background(), time.Second))
}

func chainedHelperTimeoutContext(t *testing.T) context.Context {
	return helperTimeoutContext(t)
}

func wrapOnlyContext(ctx context.Context, cancel context.CancelFunc) context.Context {
	cancel()
	return ctx
}

func doThing(ctx context.Context) {
	_ = ctx
}
