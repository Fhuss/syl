package harness

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Fhuss/syl/pkg/watcher"
)

// Runner manages execution of Harness summaries with bounded concurrency
// and in-flight request deduplication (single-flight).
type Runner struct {
	harness        Harness
	maxConcurrency int
	sem            chan struct{}
	mu             sync.Mutex
	inFlight       map[string]*call
	cache          *TitleCache
	timeout        time.Duration
}

type call struct {
	done chan struct{}
	val  string
	err  error
}

// NewRunner creates a new Runner wrapping the given Harness.
// maxConcurrency limits how many subprocesses or API calls run concurrently (default 1).
func NewRunner(h Harness, maxConcurrency int) *Runner {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	return &Runner{
		harness:        h,
		maxConcurrency: maxConcurrency,
		sem:            make(chan struct{}, maxConcurrency),
		inFlight:       make(map[string]*call),
		cache:          GetTitleCache(),
		timeout:        15 * time.Second,
	}
}

// SetTimeout configures the per-execution timeout for subprocess execution.
func (r *Runner) SetTimeout(d time.Duration) {
	if d > 0 {
		r.timeout = d
	}
}

// SetCache overrides the default cache instance (primarily for tests).
func (r *Runner) SetCache(c *TitleCache) {
	r.cache = c
}

// Harness returns the underlying Harness.
func (r *Runner) Harness() Harness {
	return r.harness
}

// Name returns the underlying harness name.
func (r *Runner) Name() string {
	if r.harness != nil {
		return r.harness.Name()
	}
	return "none"
}

// Available returns whether the underlying harness is available.
func (r *Runner) Available() bool {
	if r.harness != nil {
		return r.harness.Available()
	}
	return false
}

// Summarize fulfills the Harness interface by forwarding to SummarizeTurn.
func (r *Runner) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	return r.SummarizeTurn(ctx, "", &watcher.Turn{User: userPrompt, Assistant: assistantContent})
}

// SummarizeTurn summarizes a turn's prompt into a concise title with throttling and deduplication.
func (r *Runner) SummarizeTurn(ctx context.Context, sessionID string, turn *watcher.Turn) (string, error) {
	if r == nil || r.harness == nil || turn == nil {
		return "", fmt.Errorf("runner or turn is nil")
	}

	cache := r.cache
	if cache == nil {
		cache = GetTitleCache()
	}

	// 1. Fast cache check before any locking or queueing
	if turn.ID > 0 {
		if cached, ok := cache.Get(sessionID, turn.ID); ok && IsDefinitiveTitle(cached) {
			return cached, nil
		}
	}
	if cached, ok := cache.GetByPrompt(turn.User); ok && IsDefinitiveTitle(cached) {
		return cached, nil
	}

	// Dedup key: prefer session:turnID if turn.ID > 0, else prompt hash
	var key string
	if sessionID != "" && turn.ID > 0 {
		key = fmt.Sprintf("%s:%d", sessionID, turn.ID)
	} else {
		key = "p:" + hashPrompt(turn.User)
	}

	r.mu.Lock()
	if c, ok := r.inFlight[key]; ok {
		// Another goroutine is already executing or queued for this exact turn/prompt
		r.mu.Unlock()

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-c.done:
			return c.val, c.err
		}
	}

	c := &call{done: make(chan struct{})}
	r.inFlight[key] = c
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.inFlight, key)
		r.mu.Unlock()
		close(c.done)
	}()

	// 2. Acquire concurrency slot (queue wait)
	select {
	case <-ctx.Done():
		c.err = ctx.Err()
		return "", ctx.Err()
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	}

	// 3. Post-slot cache check (another job may have finished while this one was waiting)
	if turn.ID > 0 {
		if cached, ok := cache.Get(sessionID, turn.ID); ok && IsDefinitiveTitle(cached) {
			c.val = cached
			return cached, nil
		}
	}
	if cached, ok := cache.GetByPrompt(turn.User); ok && IsDefinitiveTitle(cached) {
		c.val = cached
		return cached, nil
	}

	// 4. Subprocess execution with isolated timeout
	execCtx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	title, err := r.harness.Summarize(execCtx, turn.User, turn.Assistant)
	if err != nil {
		c.err = err
		return "", err
	}

	if title != "" {
		if turn.ID > 0 {
			cache.Set(sessionID, turn.ID, title)
		}
		cache.SetByPrompt(turn.User, title)
	}

	c.val = title
	return title, nil
}
