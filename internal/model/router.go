package model

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Router routes requests to the appropriate model with fallback support.
type Router struct {
	models       map[string]Model
	fallback     []string
	defaultModel string

	// Rate limiting: token bucket per model
	rateLimits  map[string]*rateLimiter
	mu          sync.RWMutex

	// Cost tracking
	totalCost  float64
	totalTokens int64
	costMu     sync.Mutex
}

// rateLimiter is a simple token bucket.
type rateLimiter struct {
	tokens   float64
	maxTokens float64
	rate     float64 // tokens per second
	lastRefill time.Time
	mu       sync.Mutex
}

func newRateLimiter(maxTokens float64, rate float64) *rateLimiter {
	return &rateLimiter{
		tokens:    maxTokens,
		maxTokens: maxTokens,
		rate:      rate,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// NewRouter creates a model router.
func NewRouter(defaultModel string, fallback []string, rateLimitRPM int) *Router {
	r := &Router{
		models:       make(map[string]Model),
		fallback:     fallback,
		defaultModel: defaultModel,
		rateLimits:   make(map[string]*rateLimiter),
	}

	// Default rate limit: rateLimitRPM requests per minute per model
	rpm := float64(rateLimitRPM)
	if rpm <= 0 {
		rpm = 50
	}
	rps := rpm / 60.0
	for _, name := range fallback {
		r.rateLimits[name] = newRateLimiter(rpm, rps)
	}
	r.rateLimits[defaultModel] = newRateLimiter(rpm, rps)

	return r
}

// Register adds a model to the router.
func (r *Router) Register(m Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[m.ID()] = m
	r.rateLimits[m.ID()] = newRateLimiter(50, 50.0/60.0)
}

// Resolve finds a model by name, with fallback if not found.
func (r *Router) Resolve(requested string) Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if requested != "" {
		if m, ok := r.models[requested]; ok {
			return m
		}
	}

	// Try default
	if m, ok := r.models[r.defaultModel]; ok {
		return m
	}

	// Try fallback chain
	for _, name := range r.fallback {
		if m, ok := r.models[name]; ok {
			return m
		}
	}

	return nil
}

// CallWithFallback calls the model with automatic fallback on failure.
func (r *Router) CallWithFallback(ctx context.Context, params CallParams) (*Response, error) {
	candidates := r.candidateModels()
	if len(candidates) == 0 {
		return nil, errors.New("no models registered")
	}

	var lastErr error
	for _, m := range candidates {
		// Rate limit check
		if !r.checkRateLimit(m.ID()) {
			slog.Warn("rate limited, trying next model", "model", m.ID())
			continue
		}

		result, err := m.Call(ctx, params)
		if err == nil {
			r.trackCost(params.Messages, result.Usage)
			return result, nil
		}
		lastErr = err
		slog.Warn("model call failed, trying fallback", "model", m.ID(), "error", err)
	}

	return nil, fmt.Errorf("all models failed, last error: %w", lastErr)
}

// StreamWithFallback streams from the model with automatic fallback.
func (r *Router) StreamWithFallback(ctx context.Context, params StreamParams) (<-chan StreamEvent, error) {
	candidates := r.candidateModels()
	if len(candidates) == 0 {
		return nil, errors.New("no models registered")
	}

	// Streaming fallback is more complex - try each model
	for _, m := range candidates {
		if !r.checkRateLimit(m.ID()) {
			continue
		}

		events, err := m.Stream(ctx, params)
		if err == nil {
			return events, nil
		}
		slog.Warn("model stream failed, trying fallback", "model", m.ID(), "error", err)
	}

	return nil, errors.New("all models failed to stream")
}

// candidateModels returns models in priority order for fallback.
func (r *Router) candidateModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var result []Model

	if m, ok := r.models[r.defaultModel]; ok {
		result = append(result, m)
		seen[m.ID()] = true
	}

	for _, name := range r.fallback {
		if seen[name] {
			continue
		}
		if m, ok := r.models[name]; ok {
			result = append(result, m)
			seen[name] = true
		}
	}

	return result
}

func (r *Router) checkRateLimit(modelID string) bool {
	r.mu.RLock()
	rl, ok := r.rateLimits[modelID]
	r.mu.RUnlock()
	if !ok {
		return true
	}
	return rl.allow()
}

func (r *Router) trackCost(messages []Message, usage Usage) {
	r.costMu.Lock()
	defer r.costMu.Unlock()
	r.totalTokens += int64(usage.InputTokens + usage.OutputTokens)
	cost := float64(usage.InputTokens)*3.0/1000000 + float64(usage.OutputTokens)*15.0/1000000
	r.totalCost += cost
}

// TotalCost returns the total cost in USD.
func (r *Router) TotalCost() float64 {
	r.costMu.Lock()
	defer r.costMu.Unlock()
	return r.totalCost
}

// TotalTokens returns total token usage.
func (r *Router) TotalTokens() int64 {
	r.costMu.Lock()
	defer r.costMu.Unlock()
	return r.totalTokens
}

// ModelInfo returns metadata about registered models.
func (r *Router) ModelInfo() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info := make(map[string]string)
	for id, m := range r.models {
		thinking := "no"
		if m.SupportsThinking() {
			thinking = "yes"
		}
		info[id] = fmt.Sprintf("max_tokens=%d thinking=%s", m.MaxTokens(), thinking)
	}
	return info
}
