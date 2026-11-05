// Copyright © 2023 OpenIM open source community. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package observability

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// Context keys for tracing
	CasdoorSpanContextKey = "casdoor_span"
	CasdoorMetricsCtxKey  = "casdoor_metrics"
)

// MetricsMiddleware creates a Gin middleware for recording Casdoor metrics
func MetricsMiddleware(collector MetricsCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Store collector in context for use in handlers
		c.Set(CasdoorMetricsCtxKey, collector)
		c.Next()
	}
}

// TracingMiddleware creates a Gin middleware for distributed tracing
func TracingMiddleware(recorder SpanRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract operation type from path
		operationType := c.Request.URL.Path
		userID := c.GetString("user_id") // Populated by auth middleware
		
		span, ctx := recorder.StartSpan(c.Request.Context(), operationType, userID)
		span.Attributes = map[string]interface{}{
			"http.method":     c.Request.Method,
			"http.url":        c.Request.URL.String(),
			"http.user_agent": c.Request.UserAgent(),
		}
		
		// Update context with span
		c.Request = c.Request.WithContext(ctx)
		c.Set(CasdoorSpanContextKey, span)
		
		// Capture response
		startTime := time.Now()
		c.Next()
		
		span.Duration = time.Since(startTime)
		span.StatusCode = int32(c.Writer.Status())
		
		if c.Writer.Status() >= 400 {
			span.ErrorMessage = c.Writer.Header().Get("X-Error-Message")
		}
		
		recorder.EndSpan(span, nil)
	}
}

// GetMetricsCollector retrieves the metrics collector from context
func GetMetricsCollector(c *gin.Context) MetricsCollector {
	if collector, exists := c.Get(CasdoorMetricsCtxKey); exists {
		if mc, ok := collector.(MetricsCollector); ok {
			return mc
		}
	}
	return &NoopMetricsCollector{}
}

// GetCasdoorSpan retrieves the tracing span from context
func GetCasdoorSpan(c *gin.Context) *CasdoorSpan {
	if span, exists := c.Get(CasdoorSpanContextKey); exists {
		if cs, ok := span.(*CasdoorSpan); ok {
			return cs
		}
	}
	return &CasdoorSpan{
		Attributes: make(map[string]interface{}),
		Events:     make([]SpanEvent, 0),
	}
}

// RecordOperationMetrics is a helper to record common Casdoor operation metrics
func RecordOperationMetrics(c *gin.Context, operationType string, success bool, duration time.Duration) {
	collector := GetMetricsCollector(c)
	span := GetCasdoorSpan(c)
	
	switch operationType {
	case "login":
		collector.RecordLoginAttempt(c.Request.Context(), success, duration, nil)
	case "state_create":
		collector.RecordStateOperation(c.Request.Context(), "create", success)
	case "state_consume":
		collector.RecordStateOperation(c.Request.Context(), "consume", success)
	case "binding_create":
		collector.RecordBindingOperation(c.Request.Context(), "create", success, duration)
	case "binding_update":
		collector.RecordBindingOperation(c.Request.Context(), "update", success, duration)
	}
	
	// Add event to span
	if span != nil && span.Attributes != nil {
		span.AddEvent(operationType, map[string]interface{}{
			"success":  success,
			"duration": duration.Milliseconds(),
		})
	}
}

// Middleware to attach metrics collector to context
type MetricsContextMiddleware struct {
	collector MetricsCollector
}

func NewMetricsContextMiddleware(collector MetricsCollector) *MetricsContextMiddleware {
	return &MetricsContextMiddleware{collector: collector}
}

func (m *MetricsContextMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), CasdoorMetricsCtxKey, m.collector)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// GetMetricsFromContext extracts metrics collector from context
func GetMetricsFromContext(ctx context.Context) MetricsCollector {
	if collector, ok := ctx.Value(CasdoorMetricsCtxKey).(MetricsCollector); ok {
		return collector
	}
	return &NoopMetricsCollector{}
}
