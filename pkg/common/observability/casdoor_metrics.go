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
)

// CasdoorMetrics defines Casdoor SSO metrics for monitoring
type CasdoorMetrics struct {
	// Authentication metrics
	LoginAttempts      int64  // Total login attempts
	LoginSuccesses     int64  // Successful logins
	LoginFailures      int64  // Failed login attempts
	LoginDuration      int64  // Average login duration (ms)
	
	// OAuth state management
	StateCreated       int64  // OAuth states created
	StateConsumed      int64  // States successfully consumed
	StateExpired       int64  // States that expired
	StateValidationErr int64  // State validation errors
	
	// User binding
	BindingCreated     int64  // New Casdoor bindings created
	BindingUpdated     int64  // Bindings updated via sync
	BindingSyncErrors  int64  // Binding sync errors
	BindingSyncLatency int64  // Average sync latency (ms)
	
	// Performance
	CacheHits          int64  // Redis cache hits
	CacheMisses        int64  // Redis cache misses
	DbLatency          int64  // Database operation latency (ms)
}

// CasdoorSpan represents a tracing span for a Casdoor operation
type CasdoorSpan struct {
	OperationType   string                 // login, callback, sync, etc.
	UserID          string                 // Casdoor user ID
	StartTime       time.Time              // Operation start time
	Duration        time.Duration          // Operation duration
	StatusCode      int32                  // HTTP status or error code
	ErrorMessage    string                 // Error message if failed
	Attributes      map[string]interface{} // Custom attributes
	Events          []SpanEvent            // Timeline events
}

// AddEvent adds an event to the span
func (s *CasdoorSpan) AddEvent(name string, attributes map[string]interface{}) {
	if s == nil {
		return
	}
	s.Events = append(s.Events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attributes,
	})
}

// SpanEvent represents an event within a span
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]interface{}
}

// MetricsCollector interface for metrics collection
type MetricsCollector interface {
	RecordLoginAttempt(ctx context.Context, success bool, duration time.Duration, err error)
	RecordStateOperation(ctx context.Context, operationType string, success bool)
	RecordBindingOperation(ctx context.Context, operationType string, success bool, duration time.Duration)
	RecordCacheOperation(ctx context.Context, cacheType string, hit bool)
	RecordDbOperation(ctx context.Context, duration time.Duration)
	GetMetrics(ctx context.Context) *CasdoorMetrics
}

// SpanRecorder interface for distributed tracing
type SpanRecorder interface {
	StartSpan(ctx context.Context, operationType string, userID string) (*CasdoorSpan, context.Context)
	AddEvent(span *CasdoorSpan, eventName string, attributes map[string]interface{})
	EndSpan(span *CasdoorSpan, err error)
	RecordSpan(span *CasdoorSpan)
}

// NoopMetricsCollector is a no-op implementation for when metrics are disabled
type NoopMetricsCollector struct{}

func (n *NoopMetricsCollector) RecordLoginAttempt(ctx context.Context, success bool, duration time.Duration, err error) {}
func (n *NoopMetricsCollector) RecordStateOperation(ctx context.Context, operationType string, success bool)             {}
func (n *NoopMetricsCollector) RecordBindingOperation(ctx context.Context, operationType string, success bool, duration time.Duration) {}
func (n *NoopMetricsCollector) RecordCacheOperation(ctx context.Context, cacheType string, hit bool)                   {}
func (n *NoopMetricsCollector) RecordDbOperation(ctx context.Context, duration time.Duration)                          {}
func (n *NoopMetricsCollector) GetMetrics(ctx context.Context) *CasdoorMetrics {
	return &CasdoorMetrics{}
}

// NoopSpanRecorder is a no-op implementation for when tracing is disabled
type NoopSpanRecorder struct{}

func (n *NoopSpanRecorder) StartSpan(ctx context.Context, operationType string, userID string) (*CasdoorSpan, context.Context) {
	return &CasdoorSpan{
		OperationType: operationType,
		UserID:        userID,
		StartTime:     time.Now(),
		Attributes:    make(map[string]interface{}),
		Events:        make([]SpanEvent, 0),
	}, ctx
}
func (n *NoopSpanRecorder) AddEvent(span *CasdoorSpan, eventName string, attributes map[string]interface{}) {}
func (n *NoopSpanRecorder) EndSpan(span *CasdoorSpan, err error)                                                {}
func (n *NoopSpanRecorder) RecordSpan(span *CasdoorSpan)                                                       {}

// DefaultMetricsCollector provides in-memory metrics collection
type DefaultMetricsCollector struct {
	metrics CasdoorMetrics
}

func NewDefaultMetricsCollector() *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		metrics: CasdoorMetrics{},
	}
}

func (d *DefaultMetricsCollector) RecordLoginAttempt(ctx context.Context, success bool, duration time.Duration, err error) {
	d.metrics.LoginAttempts++
	if success {
		d.metrics.LoginSuccesses++
		d.metrics.LoginDuration = (d.metrics.LoginDuration + duration.Milliseconds()) / 2
	} else {
		d.metrics.LoginFailures++
	}
}

func (d *DefaultMetricsCollector) RecordStateOperation(ctx context.Context, operationType string, success bool) {
	switch operationType {
	case "create":
		d.metrics.StateCreated++
	case "consume":
		if success {
			d.metrics.StateConsumed++
		} else {
			d.metrics.StateValidationErr++
		}
	case "expire":
		d.metrics.StateExpired++
	}
}

func (d *DefaultMetricsCollector) RecordBindingOperation(ctx context.Context, operationType string, success bool, duration time.Duration) {
	switch operationType {
	case "create":
		d.metrics.BindingCreated++
	case "update":
		d.metrics.BindingUpdated++
		d.metrics.BindingSyncLatency = (d.metrics.BindingSyncLatency + duration.Milliseconds()) / 2
	}
	if !success {
		d.metrics.BindingSyncErrors++
	}
}

func (d *DefaultMetricsCollector) RecordCacheOperation(ctx context.Context, cacheType string, hit bool) {
	if hit {
		d.metrics.CacheHits++
	} else {
		d.metrics.CacheMisses++
	}
}

func (d *DefaultMetricsCollector) RecordDbOperation(ctx context.Context, duration time.Duration) {
	d.metrics.DbLatency = (d.metrics.DbLatency + duration.Milliseconds()) / 2
}

func (d *DefaultMetricsCollector) GetMetrics(ctx context.Context) *CasdoorMetrics {
	return &d.metrics
}
