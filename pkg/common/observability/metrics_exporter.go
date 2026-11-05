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
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// MetricsExporter handles HTTP endpoints for metrics exposure
type MetricsExporter struct {
	rtcCollector RTCMetricsCollector
	casdoorCollector MetricsCollector
	mu sync.RWMutex
}

func NewMetricsExporter(rtcCollector RTCMetricsCollector, casdoorCollector MetricsCollector) *MetricsExporter {
	return &MetricsExporter{
		rtcCollector: rtcCollector,
		casdoorCollector: casdoorCollector,
	}
}

// MetricsSnapshot represents a point-in-time snapshot of all metrics
type MetricsSnapshot struct {
	Timestamp int64 `json:"timestamp"`
	RTC *RTCMetrics `json:"rtc,omitempty"`
	Casdoor *CasdoorMetrics `json:"casdoor,omitempty"`
}

// SystemHealth represents overall system health status
type SystemHealth struct {
	Status string `json:"status"` // healthy, degraded, unhealthy
	RTC struct {
		Status string `json:"status"`
		Quality string `json:"quality"` // excellent, good, fair, poor
		AvgLatency int64 `json:"avgLatency"`
		PacketLoss float64 `json:"packetLoss"`
	} `json:"rtc"`
	Casdoor struct {
		Status string `json:"status"`
		LoginSuccessRate float64 `json:"loginSuccessRate"`
		AvgLoginTime int64 `json:"avgLoginTime"`
	} `json:"casdoor"`
}

// HandleMetrics returns current metrics snapshot
func (e *MetricsExporter) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	metrics := &MetricsSnapshot{
		Timestamp: time.Now().Unix(),
	}
	
	if e.rtcCollector != nil {
		rtcMetrics := e.rtcCollector.GetMetrics()
		metrics.RTC = rtcMetrics
	}
	
	if e.casdoorCollector != nil {
		casdoorMetrics := e.casdoorCollector.GetMetrics(r.Context())
		metrics.Casdoor = casdoorMetrics
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}

// HandleHealth returns system health status
func (e *MetricsExporter) HandleHealth(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	health := SystemHealth{
		Status: "healthy",
	}
	
	// Check RTC health
	if e.rtcCollector != nil {
		rtcMetrics := e.rtcCollector.GetMetrics()
		
		if rtcMetrics.AvgLatency > 200 || rtcMetrics.PacketLossRate > 5.0 {
			health.Status = "degraded"
		}
		
		health.RTC.Quality = assessRTCQuality(rtcMetrics)
		health.RTC.AvgLatency = rtcMetrics.AvgLatency
		health.RTC.PacketLoss = rtcMetrics.PacketLossRate
		
		if rtcMetrics.ActiveSessions > 0 {
			health.RTC.Status = "active"
		} else {
			health.RTC.Status = "idle"
		}
	}
	
	// Check Casdoor health
	if e.casdoorCollector != nil {
		casdoorMetrics := e.casdoorCollector.GetMetrics(r.Context())
		
		successRate := 0.0
		if casdoorMetrics.LoginAttempts > 0 {
			successRate = float64(casdoorMetrics.LoginSuccesses) / float64(casdoorMetrics.LoginAttempts) * 100.0
		}
		
		if successRate < 95.0 && casdoorMetrics.LoginAttempts > 10 {
			health.Status = "degraded"
		}
		
		health.Casdoor.LoginSuccessRate = successRate
		health.Casdoor.AvgLoginTime = casdoorMetrics.LoginDuration
		
		if successRate > 99.0 {
			health.Casdoor.Status = "excellent"
		} else if successRate > 95.0 {
			health.Casdoor.Status = "good"
		} else {
			health.Casdoor.Status = "poor"
		}
	}
	
	statusCode := http.StatusOK
	if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(health)
}

// HandleRTCMetrics returns RTC-specific metrics
func (e *MetricsExporter) HandleRTCMetrics(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if e.rtcCollector == nil {
		http.Error(w, "RTC metrics not available", http.StatusServiceUnavailable)
		return
	}
	
	metrics := e.rtcCollector.GetMetrics()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}

// HandleCasdoorMetrics returns Casdoor-specific metrics
func (e *MetricsExporter) HandleCasdoorMetrics(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if e.casdoorCollector == nil {
		http.Error(w, "Casdoor metrics not available", http.StatusServiceUnavailable)
		return
	}
	
	metrics := e.casdoorCollector.GetMetrics(context.Background())
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}

// PrometheusMetricsText generates Prometheus-format metrics text
func (e *MetricsExporter) PrometheusMetricsText() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	var output string
	output += "# HELP chat_rtc_active_sessions Current number of active RTC sessions\n"
	output += "# TYPE chat_rtc_active_sessions gauge\n"
	
	if e.rtcCollector != nil {
		rtcMetrics := e.rtcCollector.GetMetrics()
		output += "chat_rtc_active_sessions " + strconv.FormatInt(rtcMetrics.ActiveSessions, 10) + "\n"
		output += "# HELP chat_rtc_avg_latency Average RTC latency in milliseconds\n"
		output += "# TYPE chat_rtc_avg_latency gauge\n"
		output += "chat_rtc_avg_latency " + strconv.FormatInt(rtcMetrics.AvgLatency, 10) + "\n"
		output += "# HELP chat_rtc_packet_loss Packet loss rate percentage\n"
		output += "# TYPE chat_rtc_packet_loss gauge\n"
		output += "chat_rtc_packet_loss " + strconv.FormatFloat(rtcMetrics.PacketLossRate, 'f', 2, 64) + "\n"
	}
	
	if e.casdoorCollector != nil {
		casdoorMetrics := e.casdoorCollector.GetMetrics(context.Background())
		output += "# HELP chat_casdoor_login_attempts Total Casdoor login attempts\n"
		output += "# TYPE chat_casdoor_login_attempts counter\n"
		output += "chat_casdoor_login_attempts " + strconv.FormatInt(casdoorMetrics.LoginAttempts, 10) + "\n"
		output += "# HELP chat_casdoor_login_successes Successful Casdoor logins\n"
		output += "# TYPE chat_casdoor_login_successes counter\n"
		output += "chat_casdoor_login_successes " + strconv.FormatInt(casdoorMetrics.LoginSuccesses, 10) + "\n"
	}
	
	return output
}

// HandlePrometheus returns metrics in Prometheus format
func (e *MetricsExporter) HandlePrometheus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(e.PrometheusMetricsText()))
}

// assessRTCQuality determines overall RTC quality from metrics
func assessRTCQuality(metrics *RTCMetrics) string {
	if metrics.AvgLatency < 50 && metrics.PacketLossRate < 1.0 {
		return "excellent"
	} else if metrics.AvgLatency < 100 && metrics.PacketLossRate < 3.0 {
		return "good"
	} else if metrics.AvgLatency < 200 && metrics.PacketLossRate < 5.0 {
		return "fair"
	} else {
		return "poor"
	}
}
