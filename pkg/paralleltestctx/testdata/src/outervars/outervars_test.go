package outervars

import (
	"errors"
	"testing"
)

// Bug: outer `err` reassigned (=) inside parallel subtest.
// Mirrors https://github.com/coder/coder/pull/25980.
func TestShadowedErrWarn(t *testing.T) {
	t.Parallel()
	cases := []struct{ name string }{{"a"}, {"b"}}
	var err error
	_ = err
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err = doThing(tc.name) // want "outer-scope variable err reassigned"
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// OK: `:=` shadows.
func TestShadowedErrOK(t *testing.T) {
	t.Parallel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		_, err := doThing("a")
		_ = err
	})
}

// OK: subtest is not parallel.
func TestNonParallelSubtestOK(t *testing.T) {
	var err error
	_ = err
	t.Run("sub", func(t *testing.T) {
		_, err = doThing("a")
		_ = err
	})
}

// OK: variable is declared inside the subtest body.
func TestInnerVarOK(t *testing.T) {
	t.Parallel()
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		var err error
		_, err = doThing("a")
		_ = err
	})
}

// OK: LHS is a SelectorExpr, not an Ident.
func TestStructFieldOK(t *testing.T) {
	t.Parallel()
	state := &struct{ err error }{}
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		state.err = errors.New("x")
		_ = state.err
	})
}

// Multi-LHS: both outer vars get flagged.
func TestMultiLHSWarn(t *testing.T) {
	t.Parallel()
	var got int
	var err error
	_, _ = got, err
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		got, err = doInt() // want "outer-scope variable got reassigned" "outer-scope variable err reassigned"
		_ = got
		_ = err
	})
}

func doThing(s string) (string, error) { return s, errors.New("x") }
func doInt() (int, error)              { return 1, nil }
