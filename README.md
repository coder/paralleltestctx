# paralleltestctx

A Go static analysis tool that warns when contexts with a deadline are used in
parallel subtests.

## Problem

See [our blog post](https://coder.com/blog/go-testing-contexts-and-t-parallel)
for an in-depth explanation of the problem.

## Usage

```bash
go run github.com/coder/paralleltestctx/cmd/paralleltestctx@latest ./...
```

### Outer-scope variable reassignment

The `-check-outer-assignments` flag enables a secondary check that warns
when a variable declared in an enclosing scope is reassigned with `=`
inside a parallel subtest. Sibling parallel subtests can race on the
shared variable; the fix is almost always to shadow with `:=`.

```go
func TestBad(t *testing.T) {
   t.Parallel()
   var err error
   for _, tc := range cases {
      t.Run(tc.name, func(t *testing.T) {
         t.Parallel()
         // WARNING: `err` is shared across parallel subtests.
         _, err = doThing(tc)
         require.NoError(t, err)
      })
   }
}
```

```bash
go run github.com/coder/paralleltestctx/cmd/paralleltestctx@latest -check-outer-assignments ./...
```

The check is off by default and can be combined with `-custom-funcs`.

### Custom functions that produce contexts with timeouts

By default, the linter detects `context.WithTimeout` and `context.WithDeadline`
as producing contexts with timeouts or deadlines. Additional functions that
create a context with a deadline or timeout can be specified using the
`-custom-funcs` flag.

```bash
go run github.com/coder/paralleltestctx/cmd/paralleltestctx@latest -custom-funcs="testutil.Context" ./...
```

## Examples

### ❌ Potentially flakey test

```go
func TestBad(t *testing.T) {
   ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
   defer cancel()
   // WARNING: do not do this!
   t.Parallel()

   uut := newUnitUnderTest()
   // Danger! Context may have timed out by this point
   writeCtx(ctx, t, uut.Input, 5)
   output := readCtx(ctx, t, uut.Output)
   if output != 25 {
      t.Errorf("expected 25 got %d", output)
   }
}
```

### ❌ Potentially flakey parallel subtest

```go
func TestBad(t *testing.T) {
    t.Parallel()
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
    t.Parallel()
    ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)
    t.Cleanup(cancel)
    t.Run("sub", func(t *testing.T) {
        t.Parallel()
        // We're shadowing the parent context with a new deadline, so this is fine
        ctx, cancel := context.WithTimeout(context.Background(), time.Second * 5)
        t.Cleanup(cancel)
        doSomething(ctx)
    })
}
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file
for details.
