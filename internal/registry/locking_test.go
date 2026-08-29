package registry

import (
	"sync"
	"testing"
)

// TestConcurrentAddTestCasesNoLostWrites is the regression guard for the
// registry mutex: without r.mu, N goroutines each doing a load-mutate-save
// (AddTestCases → getBenchmark + append + saveBenchmark) race, and some
// appends are silently dropped when one goroutine's save overwrites another's.
// With the lock, every append lands. Run with -race to also catch the data
// race on the shared file.
func TestConcurrentAddTestCasesNoLostWrites(t *testing.T) {
	reg := New(t.TempDir())
	if err := reg.SaveBenchmark(Benchmark{Name: "concurrent"}); err != nil {
		t.Fatalf("SaveBenchmark() error = %v", err)
	}

	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := reg.AddTestCases("concurrent", []TestCase{{Prompt: "p"}}); err != nil {
				t.Errorf("AddTestCases() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := reg.GetBenchmark("concurrent")
	if err != nil {
		t.Fatalf("GetBenchmark() error = %v", err)
	}
	if len(got.TestCases) != n {
		t.Errorf("after %d concurrent AddTestCases, benchmark has %d test cases, want %d (writes were lost)", n, len(got.TestCases), n)
	}
}

// TestConcurrentAppendExamplesNoLostWrites is the same guard for datasets,
// whose examples file is appended to line-by-line rather than rewritten.
func TestConcurrentAppendExamplesNoLostWrites(t *testing.T) {
	reg := New(t.TempDir())
	if _, err := reg.CreateDataset("concurrent", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := reg.AppendExamples("concurrent", []Example{{Input: "in", Output: "out"}}); err != nil {
				t.Errorf("AppendExamples() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := reg.ListExamples("concurrent")
	if err != nil {
		t.Fatalf("ListExamples() error = %v", err)
	}
	if len(got) != n {
		t.Errorf("after %d concurrent AppendExamples, dataset has %d examples, want %d", n, len(got), n)
	}
}
