package harness

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fhuss/syl/pkg/watcher"
)

type mockHarness struct {
	mu          sync.Mutex
	calls       int
	activeCalls int32
	maxActive   int32
	delay       time.Duration
	result      string
	err         error
}

func (m *mockHarness) Name() string      { return "mock" }
func (m *mockHarness) Available() bool  { return true }
func (m *mockHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	curr := atomic.AddInt32(&m.activeCalls, 1)
	defer atomic.AddInt32(&m.activeCalls, -1)

	m.mu.Lock()
	m.calls++
	if curr > m.maxActive {
		m.maxActive = curr
	}
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(m.delay):
		}
	}

	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func TestRunnerBoundedConcurrency(t *testing.T) {
	mock := &mockHarness{
		delay:  30 * time.Millisecond,
		result: "Summary Result",
	}

	tmpDir := t.TempDir()
	cache := NewTitleCache(filepath.Join(tmpDir, "titles.json"))

	runner := NewRunner(mock, 1) // maxConcurrency = 1
	runner.SetCache(cache)

	const numJobs = 5
	var wg sync.WaitGroup
	wg.Add(numJobs)

	for i := 1; i <= numJobs; i++ {
		turnID := i
		go func() {
			defer wg.Done()
			turn := &watcher.Turn{
				ID:   turnID,
				User: "Prompt for turn",
			}
			_, _ = runner.SummarizeTurn(context.Background(), "session-1", turn)
		}()
	}

	wg.Wait()

	if mock.maxActive > 1 {
		t.Fatalf("expected max active calls <= 1, got %d", mock.maxActive)
	}
}

func TestRunnerDeduplication(t *testing.T) {
	mock := &mockHarness{
		delay:  40 * time.Millisecond,
		result: "Shared Topic",
	}

	tmpDir := t.TempDir()
	cache := NewTitleCache(filepath.Join(tmpDir, "titles.json"))

	runner := NewRunner(mock, 2)
	runner.SetCache(cache)

	turn := &watcher.Turn{
		ID:   42,
		User: "Deploy Redis cluster",
	}

	var wg sync.WaitGroup
	results := make([]string, 3)

	for i := 0; i < 3; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := runner.SummarizeTurn(context.Background(), "session-1", turn)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			results[idx] = res
		}()
	}

	wg.Wait()

	// All 3 callers must get the result
	for i, res := range results {
		if res != "Shared Topic" {
			t.Errorf("result[%d] = %q, want 'Shared Topic'", i, res)
		}
	}

	// Underlying harness should have been called only ONCE due to single-flight deduplication
	if mock.calls != 1 {
		t.Fatalf("expected mock.calls == 1, got %d", mock.calls)
	}
}

func TestRunnerCacheHit(t *testing.T) {
	mock := &mockHarness{
		result: "Fresh Topic",
	}

	tmpDir := t.TempDir()
	cache := NewTitleCache(filepath.Join(tmpDir, "titles.json"))
	cache.Set("session-1", 100, "Cached Topic")

	runner := NewRunner(mock, 1)
	runner.SetCache(cache)

	turn := &watcher.Turn{
		ID:   100,
		User: "Some prompt",
	}

	res, err := runner.SummarizeTurn(context.Background(), "session-1", turn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "Cached Topic" {
		t.Fatalf("expected 'Cached Topic', got %q", res)
	}

	if mock.calls != 0 {
		t.Fatalf("expected 0 mock calls on cache hit, got %d", mock.calls)
	}
}

func TestRunnerContextCancellation(t *testing.T) {
	mock := &mockHarness{
		delay:  100 * time.Millisecond,
		result: "Topic",
	}

	tmpDir := t.TempDir()
	cache := NewTitleCache(filepath.Join(tmpDir, "titles.json"))

	runner := NewRunner(mock, 1)
	runner.SetCache(cache)

	// Block the single slot with a long-running call
	go func() {
		_, _ = runner.Summarize(context.Background(), "Blocker prompt", "")
	}()

	time.Sleep(10 * time.Millisecond) // Let blocker acquire slot

	// Second call with quick cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	turn := &watcher.Turn{
		ID:   999,
		User: "Queued prompt",
	}
	_, err := runner.SummarizeTurn(ctx, "session-1", turn)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected deadline exceeded or canceled error, got %v", err)
	}
}
