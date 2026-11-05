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
	"sync"
	"sync/atomic"
	"time"
)

// RTCMetrics defines Real-Time Communication performance metrics
type RTCMetrics struct {
	// Connection metrics
	ActiveSessions      int64  // Currently active video sessions
	TotalSessions       int64  // Total sessions created
	SessionDuration     int64  // Average session duration (seconds)
	ConnectionFailures  int64  // Failed connection attempts
	
	// Latency metrics (milliseconds)
	AvgLatency          int64  // Average one-way latency (ms)
	P50Latency          int64  // 50th percentile latency (ms)
	P95Latency          int64  // 95th percentile latency (ms)
	P99Latency          int64  // 99th percentile latency (ms)
	MaxLatency          int64  // Maximum observed latency (ms)
	
	// Jitter metrics
	AvgJitter           int64  // Average jitter (ms)
	MaxJitter           int64  // Maximum jitter (ms)
	
	// Packet loss
	PacketLossRate      float64 // Packet loss percentage (0-100)
	PacketsLost         int64   // Total packets lost
	PacketsReceived     int64   // Total packets received
	
	// Bandwidth (kbps)
	AvgBitrate          int64   // Average bitrate (kbps)
	MaxBitrate          int64   // Peak bitrate (kbps)
	
	// Quality metrics
	VideoFrameRate      int32   // Average FPS
	AudioQuality        string  // Audio quality level (good/fair/poor)
	VideoQuality        string  // Video quality level (HD/SD/Low)
	NetworkQuality      string  // Network assessment (excellent/good/fair/poor)
	
	// Error metrics
	ICEErrors           int64   // WebRTC ICE connection errors
	CodecErrors         int64   // Codec encoding/decoding errors
	BufferUnderruns     int64   // Audio/video buffer underrun events
	
	// Token metrics
	TokenGenerations    int64   // Total tokens generated
	TokenErrors         int64   // Token generation errors
	AvgTokenLatency     int64   // Average token generation latency (ms)
}

// RTCSession represents a single RTC session for detailed metrics
type RTCSession struct {
	SessionID          string
	RoomID             string
	UserID             string
	PeerID             string
	StartTime          time.Time
	EndTime            time.Time
	
	LatencySamples     []int64 // Latency measurements (ms)
	JitterSamples      []int64 // Jitter measurements (ms)
	BitrateHistory     []int64 // Bitrate history (kbps)
	PacketLossHistory  []float64 // Packet loss samples
	FPSHistory        []int32  // Video frame rate history
	
	mu sync.RWMutex
}

// RTCMetricsCollector interface for RTC metrics
type RTCMetricsCollector interface {
	// Session management
	StartSession(sessionID, roomID, userID, peerID string) *RTCSession
	EndSession(sessionID string) error
	GetSession(sessionID string) *RTCSession
	
	// Latency tracking
	RecordLatency(sessionID string, latencyMs int64)
	GetLatencyStats(sessionID string) (avg, p50, p95, p99, max int64)
	
	// Jitter tracking
	RecordJitter(sessionID string, jitterMs int64)
	GetJitterStats(sessionID string) (avg, max int64)
	
	// Packet loss tracking
	RecordPacketLoss(sessionID string, lost, received int64)
	GetPacketLossRate(sessionID string) float64
	
	// Bitrate tracking
	RecordBitrate(sessionID string, bitratekbps int64)
	GetBitrateStats(sessionID string) (avg, max int64)
	
	// Video quality
	RecordFPS(sessionID string, fps int32)
	GetAverageFPS(sessionID string) int32
	
	// Quality assessment
	AssessNetworkQuality(sessionID string) string
	
	// Aggregated metrics
	GetMetrics() *RTCMetrics
	ResetMetrics()
}

// DefaultRTCMetricsCollector provides in-memory RTC metrics collection
type DefaultRTCMetricsCollector struct {
	mu           sync.RWMutex
	metrics      RTCMetrics
	sessions     map[string]*RTCSession
	sessionsMu   sync.RWMutex
}

func NewDefaultRTCMetricsCollector() *DefaultRTCMetricsCollector {
	return &DefaultRTCMetricsCollector{
		sessions: make(map[string]*RTCSession),
	}
}

func (d *DefaultRTCMetricsCollector) StartSession(sessionID, roomID, userID, peerID string) *RTCSession {
	session := &RTCSession{
		SessionID:    sessionID,
		RoomID:       roomID,
		UserID:       userID,
		PeerID:       peerID,
		StartTime:    time.Now(),
		LatencySamples: make([]int64, 0, 1000),
		JitterSamples: make([]int64, 0, 1000),
		BitrateHistory: make([]int64, 0, 100),
		PacketLossHistory: make([]float64, 0, 100),
		FPSHistory: make([]int32, 0, 100),
	}
	
	d.sessionsMu.Lock()
	d.sessions[sessionID] = session
	d.sessionsMu.Unlock()
	
	atomic.AddInt64(&d.metrics.ActiveSessions, 1)
	atomic.AddInt64(&d.metrics.TotalSessions, 1)
	
	return session
}

func (d *DefaultRTCMetricsCollector) EndSession(sessionID string) error {
	d.sessionsMu.Lock()
	session, exists := d.sessions[sessionID]
	if !exists {
		d.sessionsMu.Unlock()
		return nil
	}
	
	session.mu.Lock()
	session.EndTime = time.Now()
	session.mu.Unlock()
	
	delete(d.sessions, sessionID)
	d.sessionsMu.Unlock()
	
	atomic.AddInt64(&d.metrics.ActiveSessions, -1)
	
	// Update average session duration
	if !session.StartTime.IsZero() && !session.EndTime.IsZero() {
		duration := int64(session.EndTime.Sub(session.StartTime).Seconds())
		d.mu.Lock()
		if d.metrics.SessionDuration == 0 {
			d.metrics.SessionDuration = duration
		} else {
			d.metrics.SessionDuration = (d.metrics.SessionDuration + duration) / 2
		}
		d.mu.Unlock()
	}
	
	return nil
}

func (d *DefaultRTCMetricsCollector) GetSession(sessionID string) *RTCSession {
	d.sessionsMu.RLock()
	session, _ := d.sessions[sessionID]
	d.sessionsMu.RUnlock()
	return session
}

func (d *DefaultRTCMetricsCollector) RecordLatency(sessionID string, latencyMs int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.Lock()
	session.LatencySamples = append(session.LatencySamples, latencyMs)
	session.mu.Unlock()
	
	// Update aggregate latency
	d.mu.Lock()
	if d.metrics.AvgLatency == 0 {
		d.metrics.AvgLatency = latencyMs
	} else {
		d.metrics.AvgLatency = (d.metrics.AvgLatency + latencyMs) / 2
	}
	if latencyMs > d.metrics.MaxLatency {
		d.metrics.MaxLatency = latencyMs
	}
	d.mu.Unlock()
}

func (d *DefaultRTCMetricsCollector) GetLatencyStats(sessionID string) (avg, p50, p95, p99, max int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.RLock()
	defer session.mu.RUnlock()
	
	if len(session.LatencySamples) == 0 {
		return
	}
	
	sum := int64(0)
	for _, v := range session.LatencySamples {
		sum += v
		if v > max {
			max = v
		}
	}
	avg = sum / int64(len(session.LatencySamples))
	
	// Simplified percentile calculation
	if len(session.LatencySamples) > 0 {
		p50 = session.LatencySamples[len(session.LatencySamples)/2]
		if len(session.LatencySamples) > 1 {
			p95 = session.LatencySamples[len(session.LatencySamples)*95/100]
			p99 = session.LatencySamples[len(session.LatencySamples)*99/100]
		}
	}
	
	return
}

func (d *DefaultRTCMetricsCollector) RecordJitter(sessionID string, jitterMs int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.Lock()
	session.JitterSamples = append(session.JitterSamples, jitterMs)
	session.mu.Unlock()
	
	d.mu.Lock()
	if d.metrics.AvgJitter == 0 {
		d.metrics.AvgJitter = jitterMs
	} else {
		d.metrics.AvgJitter = (d.metrics.AvgJitter + jitterMs) / 2
	}
	if jitterMs > d.metrics.MaxJitter {
		d.metrics.MaxJitter = jitterMs
	}
	d.mu.Unlock()
}

func (d *DefaultRTCMetricsCollector) GetJitterStats(sessionID string) (avg, max int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.RLock()
	defer session.mu.RUnlock()
	
	if len(session.JitterSamples) == 0 {
		return
	}
	
	sum := int64(0)
	for _, v := range session.JitterSamples {
		sum += v
		if v > max {
			max = v
		}
	}
	avg = sum / int64(len(session.JitterSamples))
	
	return
}

func (d *DefaultRTCMetricsCollector) RecordPacketLoss(sessionID string, lost, received int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.Lock()
	if received > 0 {
		lossRate := float64(lost) / float64(received) * 100.0
		session.PacketLossHistory = append(session.PacketLossHistory, lossRate)
	}
	session.mu.Unlock()
	
	atomic.AddInt64(&d.metrics.PacketsLost, lost)
	atomic.AddInt64(&d.metrics.PacketsReceived, received)
	
	// Update aggregate loss rate
	totalReceived := atomic.LoadInt64(&d.metrics.PacketsReceived)
	totalLost := atomic.LoadInt64(&d.metrics.PacketsLost)
	if totalReceived > 0 {
		lossRate := float64(totalLost) / float64(totalReceived) * 100.0
		d.mu.Lock()
		d.metrics.PacketLossRate = lossRate
		d.mu.Unlock()
	}
}

func (d *DefaultRTCMetricsCollector) GetPacketLossRate(sessionID string) float64 {
	session := d.GetSession(sessionID)
	if session == nil {
		return 0
	}
	
	session.mu.RLock()
	defer session.mu.RUnlock()
	
	if len(session.PacketLossHistory) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, rate := range session.PacketLossHistory {
		sum += rate
	}
	return sum / float64(len(session.PacketLossHistory))
}

func (d *DefaultRTCMetricsCollector) RecordBitrate(sessionID string, bitratekbps int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.Lock()
	session.BitrateHistory = append(session.BitrateHistory, bitratekbps)
	session.mu.Unlock()
	
	d.mu.Lock()
	if d.metrics.AvgBitrate == 0 {
		d.metrics.AvgBitrate = bitratekbps
	} else {
		d.metrics.AvgBitrate = (d.metrics.AvgBitrate + bitratekbps) / 2
	}
	if bitratekbps > d.metrics.MaxBitrate {
		d.metrics.MaxBitrate = bitratekbps
	}
	d.mu.Unlock()
}

func (d *DefaultRTCMetricsCollector) GetBitrateStats(sessionID string) (avg, max int64) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.RLock()
	defer session.mu.RUnlock()
	
	if len(session.BitrateHistory) == 0 {
		return
	}
	
	sum := int64(0)
	for _, v := range session.BitrateHistory {
		sum += v
		if v > max {
			max = v
		}
	}
	avg = sum / int64(len(session.BitrateHistory))
	
	return
}

func (d *DefaultRTCMetricsCollector) RecordFPS(sessionID string, fps int32) {
	session := d.GetSession(sessionID)
	if session == nil {
		return
	}
	
	session.mu.Lock()
	session.FPSHistory = append(session.FPSHistory, fps)
	session.mu.Unlock()
	
	d.mu.Lock()
	d.metrics.VideoFrameRate = fps
	d.mu.Unlock()
}

func (d *DefaultRTCMetricsCollector) GetAverageFPS(sessionID string) int32 {
	session := d.GetSession(sessionID)
	if session == nil {
		return 0
	}
	
	session.mu.RLock()
	defer session.mu.RUnlock()
	
	if len(session.FPSHistory) == 0 {
		return 0
	}
	
	sum := int32(0)
	for _, v := range session.FPSHistory {
		sum += v
	}
	return sum / int32(len(session.FPSHistory))
}

func (d *DefaultRTCMetricsCollector) AssessNetworkQuality(sessionID string) string {
	session := d.GetSession(sessionID)
	if session == nil {
		return "unknown"
	}
	
	avg, _, _, _, _ := d.GetLatencyStats(sessionID)
	lossRate := d.GetPacketLossRate(sessionID)
	
	// Quality assessment based on metrics
	// Excellent: <50ms latency, <1% packet loss
	// Good: <100ms latency, <3% packet loss
	// Fair: <200ms latency, <5% packet loss
	// Poor: >200ms latency, >5% packet loss
	
	if avg < 50 && lossRate < 1.0 {
		return "excellent"
	} else if avg < 100 && lossRate < 3.0 {
		return "good"
	} else if avg < 200 && lossRate < 5.0 {
		return "fair"
	} else {
		return "poor"
	}
}

func (d *DefaultRTCMetricsCollector) GetMetrics() *RTCMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	m := d.metrics
	return &m
}

func (d *DefaultRTCMetricsCollector) ResetMetrics() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.metrics = RTCMetrics{}
	d.sessions = make(map[string]*RTCSession)
}

// NoOpRTCMetricsCollector provides zero-overhead metrics collection
type NoOpRTCMetricsCollector struct{}

func (n *NoOpRTCMetricsCollector) StartSession(sessionID, roomID, userID, peerID string) *RTCSession {
	return &RTCSession{SessionID: sessionID}
}

func (n *NoOpRTCMetricsCollector) EndSession(sessionID string) error { return nil }
func (n *NoOpRTCMetricsCollector) GetSession(sessionID string) *RTCSession { return nil }
func (n *NoOpRTCMetricsCollector) RecordLatency(sessionID string, latencyMs int64) {}
func (n *NoOpRTCMetricsCollector) GetLatencyStats(sessionID string) (int64, int64, int64, int64, int64) { return 0, 0, 0, 0, 0 }
func (n *NoOpRTCMetricsCollector) RecordJitter(sessionID string, jitterMs int64) {}
func (n *NoOpRTCMetricsCollector) GetJitterStats(sessionID string) (int64, int64) { return 0, 0 }
func (n *NoOpRTCMetricsCollector) RecordPacketLoss(sessionID string, lost, received int64) {}
func (n *NoOpRTCMetricsCollector) GetPacketLossRate(sessionID string) float64 { return 0 }
func (n *NoOpRTCMetricsCollector) RecordBitrate(sessionID string, bitratekbps int64) {}
func (n *NoOpRTCMetricsCollector) GetBitrateStats(sessionID string) (int64, int64) { return 0, 0 }
func (n *NoOpRTCMetricsCollector) RecordFPS(sessionID string, fps int32) {}
func (n *NoOpRTCMetricsCollector) GetAverageFPS(sessionID string) int32 { return 0 }
func (n *NoOpRTCMetricsCollector) AssessNetworkQuality(sessionID string) string { return "unknown" }
func (n *NoOpRTCMetricsCollector) GetMetrics() *RTCMetrics { return &RTCMetrics{} }
func (n *NoOpRTCMetricsCollector) ResetMetrics() {}
