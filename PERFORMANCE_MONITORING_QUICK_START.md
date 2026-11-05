# 性能监控快速开始指南

## 📊 启用性能监控

### 第一步：配置文件更新

编辑 `config/chat-rpc-chat.yml`:

```yaml
# RTC 和性能监控配置
casdoor:
  enabled: true
  enableMetrics: true        # 启用 Casdoor 指标收集
  enableTracing: false       # 暂不启用分布式追踪
  # ... 其他 Casdoor 配置

liveKit:
  url: "wss://your-livekit-server"
  key: "your-api-key"
  secret: "your-api-secret"
```

### 第二步：启动服务

```bash
# 启动 chat-api 服务
mage start

# 验证服务已启动
mage check
```

## 🔍 查看性能指标

### 方式 1: JSON 格式（推荐用于生产环境集成）

```bash
# 查看所有指标汇总
curl http://localhost:8080/api/metrics | jq .

# 查看 RTC 专用指标
curl http://localhost:8080/api/metrics/rtc | jq .

# 查看系统健康状态
curl http://localhost:8080/api/health | jq .
```

### 方式 2: Prometheus 格式（用于 Grafana 导入）

```bash
# 获取 Prometheus 格式的指标
curl http://localhost:8080/metrics

# 输出示例：
# # HELP chat_rtc_active_sessions Current number of active RTC sessions
# # TYPE chat_rtc_active_sessions gauge
# chat_rtc_active_sessions 2
# chat_rtc_avg_latency 85
# chat_rtc_packet_loss 0.50
```

## 📈 关键指标解读

### RTC 指标

```json
{
  "rtc": {
    "activeSessions": 2,           // 当前活跃会话数
    "totalSessions": 128,          // 总会话数
    "sessionDuration": 3600,       // 平均会话时长（秒）
    
    "avgLatency": 85,              // 🔴 关键：平均延迟（毫秒）
    "p50Latency": 70,              // 中位数延迟
    "p95Latency": 150,             // 95% 用户的延迟
    "p99Latency": 200,             // 99% 用户的延迟
    "maxLatency": 280,             // 最大延迟
    
    "avgJitter": 12,               // 🟡 重要：抖动（毫秒）
    "maxJitter": 45,
    
    "packetLossRate": 0.5,         // 🔴 关键：丢包率（百分比）
    "packetsLost": 1200,
    "packetsReceived": 240000,
    
    "avgBitrate": 2500,            // 🟡 重要：码率（kbps）
    "maxBitrate": 4000,
    
    "videoFrameRate": 30,          // 🟡 重要：帧率（fps）
    "audioQuality": "good",        // good/fair/poor
    "videoQuality": "HD",          // HD/SD/Low
    "networkQuality": "good"       // excellent/good/fair/poor
  }
}
```

### 健康状态解读

```json
{
  "status": "healthy",    // healthy/degraded/unhealthy
  "rtc": {
    "status": "active",
    "quality": "good",
    "avgLatency": 85,
    "packetLoss": 0.5
  }
}
```

**健康判断逻辑：**
- ✅ **健康** (healthy): 延迟 < 150ms 且丢包 < 3%
- ⚠️  **降级** (degraded): 延迟 150-200ms 或丢包 3-5%
- ❌ **不健康** (unhealthy): 延迟 > 200ms 且丢包 > 5%

## 🎯 快速诊断流程

### 问题：延迟过高 (> 150ms)

```bash
# 1. 检查当前指标
curl http://localhost:8080/api/metrics/rtc | jq '.avgLatency, .p95Latency'

# 2. 检查网络延迟
ping -c 10 your-livekit-server

# 3. 检查丢包
curl http://localhost:8080/api/metrics/rtc | jq '.packetLossRate'

# 4. 检查帧率
curl http://localhost:8080/api/metrics/rtc | jq '.videoFrameRate'

# 5. 建议
如果 ping 延迟 > 100ms，属于网络/地理位置问题
如果帧率 < 24fps，降低分辨率或码率
如果丢包 > 2%，启用 FEC 冗余编码
```

### 问题：频繁卡顿（高抖动）

```bash
# 1. 检查抖动
curl http://localhost:8080/api/metrics/rtc | jq '.avgJitter, .maxJitter'

# 2. 检查丢包
curl http://localhost:8080/api/metrics/rtc | jq '.packetLossRate'

# 3. 检查缓冲
# 增加 jitterBufferSize 以平衡延迟和稳定性

# 4. 建议
如果 maxJitter > 50ms，启用 NACK 重传机制
如果丢包 > 1%，启用 FEC 冗余
```

### 问题：码率不足

```bash
# 1. 检查码率
curl http://localhost:8080/api/metrics/rtc | jq '.avgBitrate, .maxBitrate'

# 2. 检查帧率
curl http://localhost:8080/api/metrics/rtc | jq '.videoFrameRate'

# 3. 检查丢包
curl http://localhost:8080/api/metrics/rtc | jq '.packetLossRate'

# 4. 建议
如果码率 < 1500kbps，检查网络带宽
如果帧率 < 24fps，启用自适应编码
如果丢包高，降低码率避免进一步恶化
```

## 📊 集成 Grafana 仪表板

### 第一步：配置 Prometheus

编辑 `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'chat-api'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 5s  # RTC 指标需要更频繁采集
```

### 第二步：启动 Prometheus

```bash
prometheus --config.file=prometheus.yml
```

### 第三步：在 Grafana 中创建仪表板

```
1. 访问 http://localhost:3000 (Grafana)
2. 添加 Prometheus 数据源: http://localhost:9090
3. 创建新仪表板
4. 导入以下预置图表：

查询示例：
- 活跃会话: chat_rtc_active_sessions
- 平均延迟: chat_rtc_avg_latency
- 丢包率: chat_rtc_packet_loss
- 成功率: rate(chat_casdoor_login_successes[5m]) / rate(chat_casdoor_login_attempts[5m])
```

## 🔧 性能优化建议

基于监控指标的优化优先级：

| 指标 | 目标 | 当前 | 优先级 | 优化方案 |
|------|------|------|--------|--------|
| 延迟 | < 100ms | 150ms | 🔴 高 | 检查网络/使用边缘节点 |
| 抖动 | < 20ms | 40ms | 🔴 高 | 启用 NACK/FEC |
| 丢包率 | < 1% | 2.5% | 🔴 高 | 启用冗余编码 |
| 帧率 | 30fps | 24fps | 🟡 中 | 优化编码器 |
| 码率 | 2500kbps | 2000kbps | 🟡 中 | 增加带宽或启用自适应 |

## 🚨 告警配置示例

### Prometheus 告警规则

```yaml
# alert-rules.yml
groups:
- name: rtc_alerts
  interval: 1m
  rules:
  
  # 高延迟告警
  - alert: RTCHighLatency
    expr: chat_rtc_avg_latency > 150
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "RTC 平均延迟过高"
      description: "当前延迟: {{ $value }}ms (> 150ms)"
  
  # 高丢包告警
  - alert: RTCHighPacketLoss
    expr: chat_rtc_packet_loss > 5
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "RTC 丢包率过高"
      description: "当前丢包率: {{ $value }}%（> 5%）"
  
  # 低帧率告警
  - alert: RTCLowFrameRate
    expr: chat_rtc_video_frame_rate < 15
    for: 3m
    labels:
      severity: warning
    annotations:
      summary: "RTC 视频帧率过低"
      description: "当前帧率: {{ $value }}fps（< 15fps）"
  
  # Casdoor 登录失败率高
  - alert: CasdoorHighFailureRate
    expr: (1 - (rate(chat_casdoor_login_successes[5m]) / rate(chat_casdoor_login_attempts[5m]))) > 0.05
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Casdoor 登录失败率过高"
      description: "失败率: {{ $value | humanizePercentage }}"
```

## 📞 常见问题排查

### Q: 指标端点无法访问
A: 检查服务是否启动，确认 `enableMetrics: true`
```bash
curl -v http://localhost:8080/api/metrics
```

### Q: 指标数据一直为 0
A: 需要先有 RTC 会话活动，指标才会更新
```bash
# 发起一个视频会话，然后查看指标
curl http://localhost:8080/api/metrics/rtc
```

### Q: 如何持久化指标数据？
A: 在 config/chat-rpc-chat.yml 中启用 Prometheus 导出器（Phase 3）

### Q: 指标数据何时重置？
A: 服务重启时重置；生产环境应使用 Prometheus 长期存储

## 🔗 相关文档

- [RTC 性能优化完整指南](./RTC_PERFORMANCE_GUIDE.md)
- [Casdoor 集成文档](./CASDOOR_INTEGRATION_COMPLETE.md)
- [可观测性架构设计](./IMPROVEMENTS_DETAIL.md)

---

**最后更新**: 2026-11-05  
**维护者**: OpenIM 团队
