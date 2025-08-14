package paralleltestctx

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestBasic(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer(), "basic")
}

func TestCustomNoReceiver(t *testing.T) {
	testdata := analysistest.TestData()

	// Test with additional custom timeout functions (standard ones are always included)
	analyzer := newCtxAnalyzer()
	analyzer.timeoutFuncFlag = "Context,TestutilWithTimeout,factory.CreateContext"

	analysistest.Run(t, testdata, analyzer.analyzer, "custom")
}

func TestCustomWithReceiver(t *testing.T) {
	testdata := analysistest.TestData()

	analyzer := newCtxAnalyzer()
	analyzer.timeoutFuncFlag = "testutil.Context,foo.Context,factory.CreateContext,TestutilWithTimeout"

	analysistest.Run(t, testdata, analyzer.analyzer, "custom")
}
