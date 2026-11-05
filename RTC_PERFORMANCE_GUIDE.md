# RTC 视频通话延迟优化指南

## 📊 性能监控系统

### 1. 查看实时指标

#### A. 性能监控端点（需在 start.go 中配置）

```bash
# 查看所有指标汇总
curl http://localhost:8080/api/metrics

# 查看 RTC 专用指标
curl http://localhost:8080/api/metrics/rtc

# 查看系统健康状态
curl http://localhost:8080/api/health

# Prometheus 格式指标（用于导入 Grafana）
curl http://localhost:8080/metrics
```

#### B. 指标响应示例

```json
{
  "timestamp": 1730803524,
  "rtc": {
    "activeSessions": 5,
    "totalSessions": 128,
    "sessionDuration": 3600,
    "avgLatency": 85,        // 关键：平均延迟（毫秒）
    "p50Latency": 70,
    "p95Latency": 150,
    "p99Latency": 200,
    "maxLatency": 280,
    "avgJitter": 12,         // 关键：平均抖动（毫秒）
    "maxJitter": 45,
    "packetLossRate": 0.5,   // 关键：丢包率（百分比）
    "packetsLost": 1200,
    "packetsReceived": 240000,
    "avgBitrate": 2500,      // 关键：平均码率（kbps）
    "maxBitrate": 4000,
    "videoFrameRate": 30,    // 关键：帧率（fps）
    "audioQuality": "good",
    "videoQuality": "HD",
    "networkQuality": "good"
  }
}
```

### 2. 健康检查

```bash
curl http://localhost:8080/api/health -s | jq .

# 响应示例
{
  "status": "healthy",
  "rtc": {
    "status": "active",
    "quality": "good",
    "avgLatency": 85,
    "packetLoss": 0.5
  },
  "casdoor": {
    "status": "excellent",
    "loginSuccessRate": 99.8,
    "avgLoginTime": 320
  }
}
```

## 🔧 RTC 延迟优化方案

### 三个延迟来源

```
总延迟 = 编码延迟 + 网络延迟 + 解码延迟 + 缓冲延迟
         ≈ 30-50ms  + 80-150ms  + 20-30ms   + 10-50ms
       = 140-280ms 标准范围
```

### 性能目标

| 等级 | 指标 | 标准 |
|------|------|------|
| **优秀** | 延迟 | < 50ms |
| | 抖动 | < 10ms |
| | 丢包率 | < 1% |
| **良好** | 延迟 | < 100ms |
| | 抖动 | < 20ms |
| | 丢包率 | < 3% |
| **一般** | 延迟 | 100-200ms |
| | 抖动 | 20-50ms |
| | 丢包率 | 3-5% |
| **较差** | 延迟 | > 200ms |
| | 抖动 | > 50ms |
| | 丢包率 | > 5% |

## 🚀 优化策略

### 1. **编码优化**（30-50ms）

如果 `videoFrameRate < 24`，尝试以下方案：

```go
// config/chat-rpc-chat.yml 配置示例
liveKit:
  url: "wss://livekit.example.com"
  key: "your-api-key"
  secret: "your-api-secret"
  
  # 视频编码优化
  videoCodec: "vp9"          # 优于 h264，质量更好
  maxBitrate: 4000           # 最大码率 (kbps)
  initialBitrate: 2500       # 初始码率 (kbps)
  videoWidth: 1280           # 分辨率
  videoHeight: 720           # 分辨率
  videoFramerate: 30         # 帧率 (fps)
  
  # 音频编码优化
  audioCodec: "opus"         # 推荐 Opus
  audioChannels: 1           # 单声道降低延迟
  audioBitrate: 128          # 音频码率 (kbps)
  
  # 网络优化
  enableDtx: true            # DTX (不传输静音) 降低带宽
  enableRed: true            # 冗余编码，降低丢包影响
```

### 2. **网络优化**（80-150ms）

#### A. 诊断当前网络质量

```bash
# 使用客户端工具测试网络到 LiveKit 服务器的延迟
ping livekit.example.com
traceroute livekit.example.com

# 如果平均延迟 > 150ms，说明跨地域延迟是主要问题
```

#### B. 网络层优化

```go
// 启用以下特性
liveKit:
  # ICE 连接优化
  enableTcpRelay: true       # 允许 TCP 中继（缓慢网络）
  preferredCodecMime: "video/vp9"
  
  # 带宽自适应
  enableBandwidthProbing: true
  enableDynacast: true       # 动态编码多个质量版本
  
  # 网络恢复
  enableNetworkAdaption: true # 自动降低质量以应对网络变化
  reconnectPolicy: "exponential" # 重连策略
  maxReconnectAttempts: 5
```

#### C. 地理位置优化

```
如果全球用户延迟都很高：
1. 部署边缘节点（各地区 LiveKit 实例）
   - 中国: CDN 节点 + 阿里云/腾讯云
   - 欧美: AWS/Azure 区域节点
   
2. 配置客户端自动选择最近的节点
   regions: [
     "asia-cn",    # 中国用户
     "asia-sg",    # 东南亚
     "us-west",    # 美国西部
     "eu-central"  # 欧洲
   ]
```

### 3. **缓冲优化**（10-50ms）

```go
// internal/api/chat/rtc.go - 添加缓冲控制
type RTCConfig struct {
    JitterBufferSize int    // 抖动缓冲大小 (ms)
    AudioPlayoutDelay int  // 音频播放延迟 (ms)
    MaxPlayoutDelay int    // 最大播放延迟 (ms)
}

// 推荐值
const (
    OptimalJitterBuffer = 50  // 50ms 缓冲
    ReducedJitterBuffer = 20  // 低延迟模式
    LowBandwidthBuffer  = 100 // 弱网环境
)
```

### 4. **丢包恢复**（关键）

```go
// 当 packetLossRate > 3% 时应用以下策略

// 方案 1: FEC（前向纠错）
liveKit:
  enableRed: true            # Redundant Encoding
  redUlpfecPayloadType: 120  # RTP 负载类型

// 方案 2: RTX（重传）
liveKit:
  enableRtx: true            # 启用重传
  rtxPayloadType: 96

// 方案 3: 适应码率
type AdaptiveCodec struct {
    MaxBitrate int  // 根据丢包率动态调整
    MinBitrate int
}
```

## 📈 监控 + 告警

### 1. 关键指标监控

```yaml
# Prometheus 告警规则示例 (prometheus-rules.yml)
groups:
- name: rtc_alerts
  rules:
  # 高延迟告警
  - alert: RTCHighLatency
    expr: chat_rtc_avg_latency > 150
    for: 5m
    annotations:
      summary: "RTC 平均延迟过高: {{ $value }}ms"
      
  # 高丢包告警
  - alert: RTCHighPacketLoss
    expr: chat_rtc_packet_loss > 5
    for: 2m
    annotations:
      summary: "RTC 丢包率过高: {{ $value }}%"
      
  # 低帧率告警
  - alert: RTCLowFrameRate
    expr: chat_rtc_video_frame_rate < 15
    for: 3m
    annotations:
      summary: "RTC 帧率过低: {{ $value }} fps"
```

### 2. 日志追踪

```bash
# 启用 RTC 调试日志
export CHAT_LOG_LEVEL=debug

# 查看具体会话的性能数据
curl http://localhost:8080/api/sessions/{sessionID}/metrics
```

## 🐛 常见问题诊断

### 问题 1: 延迟 150ms+

```bash
# 诊断步骤
1. 检查网络延迟
   ping -c 10 livekit.example.com
   # 如果 > 100ms，说明网络/地理位置是问题

2. 检查丢包
   curl http://localhost:8080/api/metrics/rtc | jq .packetLossRate
   # 如果 > 2%，启用 FEC/RTX

3. 检查编码
   curl http://localhost:8080/api/metrics/rtc | jq .videoFrameRate
   # 如果 < 24fps，降低分辨率或码率

4. 检查缓冲
   # 减小 jitterBufferSize 配置
```

### 问题 2: 频繁卡顿 (高抖动)

```bash
# 症状：avgJitter > 30ms
curl http://localhost:8080/api/metrics/rtc | jq .{avgJitter,maxJitter}

# 解决方案
1. 检查服务器 CPU 使用率（编码压力）
2. 检查带宽抖动（网络质量问题）
3. 启用 NACK 重传: enableRtx: true
4. 增加缓冲: jitterBufferSize: 100
```

### 问题 3: 音视频不同步

```bash
# 原因：播放延迟差异过大
# 解决方案
1. 同步音视频解码缓冲大小
2. 使用 NTP 时钟同步
3. 调整 audioPlayoutDelay 匹配视频
```

## 📋 实施检查清单

- [ ] 启用性能监控（`/api/metrics` 端点）
- [ ] 配置基线指标收集
- [ ] 设置告警规则（延迟 > 150ms, 丢包 > 3%）
- [ ] 优化编码配置（分辨率、码率、帧率）
- [ ] 检查网络延迟（Ping 测试）
- [ ] 启用 FEC/RTX（处理丢包）
- [ ] 测试全球各地区延迟
- [ ] 配置 Grafana 仪表板
- [ ] 建立性能基准线
- [ ] 定期审查性能指标

## 🔗 相关资源

- [WebRTC 性能最佳实践](https://www.w3.org/TR/webrtc-stats/)
- [LiveKit 文档](https://docs.livekit.io/)
- [Prometheus 告警管理](https://prometheus.io/docs/alerting/latest/overview/)
- [Grafana 仪表板创建](https://grafana.com/docs/grafana/latest/dashboards/)

## 下一步

### Phase 3 中要完成的工作

1. **指标导出器集成**
   ```go
   // pkg/common/observability/prometheus_exporter.go
   // 集成官方 Prometheus Go 客户端库
   // 暴露完整的指标集到 /metrics 端点
   ```

2. **Grafana 仪表板**
   - 实时 RTC 性能仪表板
   - 历史趋势分析
   - 告警状态视图

3. **分布式追踪**
   ```go
   // 使用 Jaeger 追踪单个会话的完整调用链
   // 快速定位延迟瓶颈
   ```

4. **自动化诊断**
   - 异常检测告警
   - 自动降级建议
   - 性能报告生成

---

**更新时间**: 2026-11-05  
**版本**: 1.0  
**维护者**: OpenIM 团队
