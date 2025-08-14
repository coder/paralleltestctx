# paralleltestctx

A Go static analysis tool that warns when timeout contexts are used after `t.Parallel()` calls in tests.

## Problem

See [our blog post](https://coder.com/blog/go-testing-contexts-and-t-parallel) for an in-depth explanation of the problem.

## Usage

```bash
go run github.com/coder/paralleltestctx/cmd/paralleltestctx@latest ./...
```

### Custom functions that produce contexts with timeouts

By default, detects `context.WithTimeout` and `context.WithDeadline`.
Additional functions that create a context with a deadline or timeout can be
specified using the `-custom-funcs` flag.

```bash
go run github.com/coder/paralleltestctx/cmd/paralleltestctx@latest -custom-funcs="testutil.Context" ./...
```

## Examples

### ❌ Potentially flakey test

```go
func TestBad(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)
    t.Cleanup(cancel)
    t.Run("sub", func(t *testing.T) {
        t.Parallel()
        // Danger! Context may have timed out by this point
        doSomething(ctx) // Warning: timeout context used after t.Parallel call
    })
}
```

### ✅ Fixed test - create a new context

```go
func TestGood(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)
    t.Cleanup(cancel)
    t.Run("sub", func(t *testing.T) {
        t.Parallel()
        ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)
        t.Cleanup(cancel)
        doSomething(ctx)
    })
}
```
