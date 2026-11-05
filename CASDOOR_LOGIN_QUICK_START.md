# Casdoor 登录对接 - 快速模板

> 🚀 5分钟快速开始

## 1️⃣ Casdoor 配置（2分钟）

### A. 创建应用

在 Casdoor 管理面板 (http://casdoor:8000/admin)：

```
应用 → 添加
├─ 名称: OpenIM Chat
├─ 重定向 URI: http://localhost:8888/account/casdoor/callback
└─ 提供商: 默认 (Casdoor)
```

### B. 获取凭证

```
Client ID:     xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
Client Secret: yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy
```

---

## 2️⃣ 服务器配置（1分钟）

编辑 `config/chat-rpc-chat.yml`:

```yaml
casdoor:
  enabled: true
  endpoint: "http://casdoor-server:8000"
  clientId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"     # 粘贴 Client ID
  clientSecret: "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy" # 粘贴 Client Secret
  syncSecret: "your-webhook-secret-12345"
  redisCache: true
  enableMetrics: true
```

启动：
```bash
mage start
mage check  # 验证
```

---

## 3️⃣ 前端对接（2分钟）

### A. 登录按钮

```html
<button onclick="loginWithCasdoor()">
  🔐 Casdoor 登录
</button>
```

### B. JavaScript 代码

```javascript
// ==========================================
// Casdoor OAuth 2.0 登录流程
// ==========================================

// 1. 获取登录 URL
async function loginWithCasdoor() {
  const response = await fetch('/account/casdoor/login_url', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      client: 'web',
      platform: 3,  // Web
      redirectURI: window.location.origin + '/auth/callback'
    })
  });
  
  const { data } = await response.json();
  
  // 保存 state 防止 CSRF
  sessionStorage.setItem('casdoor_state', data.state);
  
  // 重定向到 Casdoor
  window.location.href = data.url;
}

// 2. 处理回调（在 /auth/callback 页面）
async function handleCallback() {
  const params = new URLSearchParams(location.search);
  const code = params.get('code');
  const state = params.get('state');
  
  // 验证 state
  if (state !== sessionStorage.getItem('casdoor_state')) {
    alert('安全检查失败！');
    return;
  }
  
  // 发送 code 到服务器
  const response = await fetch('/account/casdoor/callback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      code: code,
      state: state,
      redirectURI: window.location.origin + '/auth/callback',
      platform: 3,
      deviceID: getOrCreateDeviceID()
    })
  });
  
  const result = await response.json();
  
  if (result.code !== 0) {
    alert('登录失败: ' + result.msg);
    return;
  }
  
  // 保存 Token
  const { userID, chatToken, imToken } = result.data;
  localStorage.setItem('userID', userID);
  localStorage.setItem('chatToken', chatToken);
  localStorage.setItem('imToken', imToken);
  
  // 清理临时数据
  sessionStorage.removeItem('casdoor_state');
  
  // 跳转到主页
  window.location.href = '/';
}

// 3. 登出
async function logout() {
  const response = await fetch('/account/casdoor/logout', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + localStorage.getItem('chatToken')
    },
    body: JSON.stringify({
      redirectURI: window.location.origin + '/login'
    })
  });
  
  const result = await response.json();
  
  // 清理 Token
  localStorage.clear();
  
  // 跳转（可能到 Casdoor 登出页）
  if (result.data?.logoutURL) {
    window.location.href = result.data.logoutURL;
  } else {
    window.location.href = '/login';
  }
}

// 辅助函数
function getOrCreateDeviceID() {
  let id = localStorage.getItem('deviceID');
  if (!id) {
    id = 'device_' + Math.random().toString(36).substr(2, 9);
    localStorage.setItem('deviceID', id);
  }
  return id;
}

// 页面加载时检查回调
if (location.pathname === '/auth/callback') {
  handleCallback();
}
```

### C. 可选：密码登录

```javascript
async function loginWithPassword(username, password) {
  const response = await fetch('/account/casdoor/password', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: username,
      password: password,
      platform: 3,
      deviceID: getOrCreateDeviceID()
    })
  });
  
  const result = await response.json();
  
  if (result.code !== 0) {
    alert('登录失败: ' + result.msg);
    return;
  }
  
  // 保存 Token
  const { userID, chatToken, imToken } = result.data;
  localStorage.setItem('userID', userID);
  localStorage.setItem('chatToken', chatToken);
  localStorage.setItem('imToken', imToken);
  
  // 跳转
  window.location.href = '/';
}
```

---

## 📡 API 参考

### 1. 获取登录 URL

```bash
curl -X POST http://localhost:8080/account/casdoor/login_url \
  -H 'Content-Type: application/json' \
  -d '{
    "client": "web",
    "platform": 3,
    "redirectURI": "http://localhost:3000/auth/callback"
  }'
```

**响应**:
```json
{
  "code": 0,
  "data": {
    "url": "http://casdoor:8000/login/oauth/authorize?client_id=xxx&...",
    "state": "abc123def456"
  }
}
```

### 2. 处理回调

```bash
curl -X POST http://localhost:8080/account/casdoor/callback \
  -H 'Content-Type: application/json' \
  -d '{
    "code": "auth_code_from_casdoor",
    "state": "abc123def456",
    "redirectURI": "http://localhost:3000/auth/callback",
    "platform": 3,
    "deviceID": "device_123"
  }'
```

**响应**:
```json
{
  "code": 0,
  "data": {
    "userID": "user_abc123",
    "chatToken": "eyJhbGc...",
    "imToken": "eyJhbGc..."
  }
}
```

### 3. 密码登录

```bash
curl -X POST http://localhost:8080/account/casdoor/password \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "user@example.com",
    "password": "password123",
    "platform": 3,
    "deviceID": "device_123"
  }'
```

### 4. 登出

```bash
curl -X POST http://localhost:8080/account/casdoor/logout \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer chatToken' \
  -d '{
    "redirectURI": "http://localhost:3000/login"
  }'
```

---

## 🧪 测试

### A. 本地测试

```bash
# 1. 启动服务
mage start

# 2. 检查 Casdoor 配置
curl http://localhost:8080/api/health | jq '.casdoor'

# 3. 测试登录 URL 生成
curl -X POST http://localhost:8080/account/casdoor/login_url \
  -H 'Content-Type: application/json' \
  -d '{"client":"web","platform":3,"redirectURI":"http://localhost:3000/callback"}'

# 4. 检查日志
tail -f logs/chat.log | grep -i casdoor
```

### B. 端到端测试

1. 打开浏览器：http://localhost:3000/login
2. 点击"Casdoor 登录"按钮
3. 在 Casdoor 输入用户名密码
4. 应该重定向回应用并自动登录
5. 检查 localStorage 中是否有 Token

---

## 🔧 配置参考

```yaml
# config/chat-rpc-chat.yml 完整示例

casdoor:
  # 启用/禁用
  enabled: true
  
  # Casdoor 服务器地址
  # 生产: https://casdoor.your-domain.com
  # 本地: http://localhost:8000
  endpoint: "http://localhost:8000"
  
  # 从 Casdoor 应用获取
  clientId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  clientSecret: "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"
  
  # 用于 Webhook 验证
  syncSecret: "your-webhook-secret-12345"
  
  # 缓存配置
  redisCache: true        # 多实例必须启用
  
  # 可观测性
  enableMetrics: true     # 启用性能监控
  enableTracing: false    # Jaeger 追踪 (Phase 3)

# Redis 配置（如果启用 redisCache）
redis:
  host: "127.0.0.1"
  port: 6379
  password: ""
  db: 0
  dialTimeout: "10s"
  readTimeout: "3s"
  writeTimeout: "3s"
```

---

## ⚠️ 常见错误及解决

| 错误 | 原因 | 解决 |
|------|------|------|
| `redirect_uri_mismatch` | 重定向 URI 不匹配 | 检查 Casdoor 应用配置 |
| `invalid_code` | 授权码无效或过期 | 重新登录，code 有效期 10 分钟 |
| `state_invalid` | State 验证失败 | 检查 Redis 或浏览器 Cookie |
| `user_not_found` | 用户信息获取失败 | 检查 Casdoor 用户是否完整 |
| `connection_refused` | 无法连接 Casdoor | 检查 Casdoor URL 和网络 |

---

## 📊 监控登录

```bash
# 查看 Casdoor 登录指标
curl http://localhost:8080/api/metrics/casdoor | jq '{
  loginAttempts: .loginAttempts,
  loginSuccesses: .loginSuccesses,
  loginFailures: .loginFailures,
  avgLoginTime: .avgLoginTime
}'

# 示例输出
{
  "loginAttempts": 42,
  "loginSuccesses": 40,
  "loginFailures": 2,
  "avgLoginTime": 320
}
```

---

## 🚀 部署到生产

### Docker Compose

```yaml
version: '3.8'

services:
  casdoor:
    image: casbin/casdoor:latest
    ports:
      - "8000:8000"
    environment:
      DB_DRIVER: postgres
      DB_DATA_SOURCE: "postgres://user:pass@db:5432/casdoor"

  chat-api:
    build: .
    ports:
      - "8080:8080"
    environment:
      CHATENV_CHAT_CASDOOR_ENABLED: "true"
      CHATENV_CHAT_CASDOOR_ENDPOINT: "http://casdoor:8000"
      CHATENV_CHAT_CASDOOR_CLIENT_ID: "your-client-id"
      CHATENV_CHAT_CASDOOR_CLIENT_SECRET: "your-client-secret"
      CHATENV_CHAT_CASDOOR_SYNC_SECRET: "your-webhook-secret"
    depends_on:
      - casdoor

  redis:
    image: redis:latest
    ports:
      - "6379:6379"
```

### Kubernetes

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chat-config
data:
  chat-rpc-chat.yml: |
    casdoor:
      enabled: true
      endpoint: "http://casdoor-service:8000"
      clientId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      clientSecret: "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"
      syncSecret: "your-webhook-secret"
      redisCache: true
      enableMetrics: true
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chat-api
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: chat-api
        image: openim/chat:latest
        volumeMounts:
        - name: config
          mountPath: /etc/chat
      volumes:
      - name: config
        configMap:
          name: chat-config
```

---

## ✅ 验证清单

- [ ] Casdoor 服务已启动
- [ ] 应用已创建，Client ID 和 Secret 已记录
- [ ] 重定向 URI 已配置
- [ ] config/chat-rpc-chat.yml 已更新
- [ ] Redis 已启动（如启用 redisCache）
- [ ] Chat API 已启动
- [ ] 前端代码已集成
- [ ] 测试登录流程
- [ ] 测试密码登录（可选）
- [ ] 测试登出流程
- [ ] 检查性能指标
- [ ] 配置监控告警

---

**快速开始时间**: ⏱️ ~5 分钟  
**完整部署时间**: ⏱️ ~30 分钟  
**更新时间**: 2026-11-05
