package custom

import (
	"context"
	"testing"
	"time"
)

type foo struct{}

// Custom timeout function that should be detected when configured
func (foo) Context(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// Another custom timeout function
func TestutilWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func TestCustomTimeoutWarn(t *testing.T) {
	ctx, cancel := (foo{}).Context(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

func TestTestutilTimeoutWarn(t *testing.T) {
	ctx, cancel := TestutilWithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

// Test that standard functions still work
func TestStandardTimeoutWarn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

// Test function that should NOT be detected by default
func NotATimeoutFunc(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func TestNotDetectedByDefault(t *testing.T) {
	ctx, cancel := NotATimeoutFunc(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel() // should not warn when using default config
		_ = ctx
	})
}

// Test package-level function call (simulating testutil.Context)
func TestPackageTimeoutWarn(t *testing.T) {
	ctx, cancel := testutil.Context(context.Background(), time.Second) // Simulates testutil.Context()
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

// Simulate the actual testutil package behavior
type testutilType struct{}

func (testutilType) Context(parent context.Context, dur time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dur)
}

var testutil = testutilType{}

// Test direct composite literal method calls: foo{}.Method()
func TestDirectCompositeLiteral(t *testing.T) {
	ctx, cancel := foo{}.Context(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}

// Test direct composite literal method calls without parentheses: factory{}.Method()
type factory struct{}

func (factory) CreateContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func TestFactoryPattern(t *testing.T) {
	ctx, cancel := factory{}.CreateContext(context.Background(), time.Second)
	defer cancel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_ = ctx // want "timeout context ctx used after a t.Parallel call"
	})
}
