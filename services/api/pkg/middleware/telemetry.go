package middleware

import (
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type HTTPSnapshot struct {
	Requests  int64   `json:"requests"`
	Errors5xx int64   `json:"errors_5xx"`
	P95MS     float64 `json:"p95_ms"`
}

type HTTPRegistry struct {
	mu        sync.Mutex
	requests  int64
	errors5xx int64
	durations []float64
}

func NewHTTPRegistry() *HTTPRegistry { return &HTTPRegistry{durations: make([]float64, 0, 512)} }

func (registry *HTTPRegistry) Observe() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		started := time.Now()
		ctx.Next()
		elapsed := float64(time.Since(started).Microseconds()) / 1000
		registry.mu.Lock()
		defer registry.mu.Unlock()
		registry.requests++
		if ctx.Writer.Status() >= 500 {
			registry.errors5xx++
		}
		if len(registry.durations) == cap(registry.durations) {
			copy(registry.durations, registry.durations[1:])
			registry.durations = registry.durations[:len(registry.durations)-1]
		}
		registry.durations = append(registry.durations, elapsed)
	}
}

func (registry *HTTPRegistry) Snapshot() HTTPSnapshot {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	values := append([]float64(nil), registry.durations...)
	sort.Float64s(values)
	p95 := 0.0
	if len(values) > 0 {
		index := (95*len(values)+99)/100 - 1
		p95 = values[index]
	}
	return HTTPSnapshot{Requests: registry.requests, Errors5xx: registry.errors5xx, P95MS: p95}
}
