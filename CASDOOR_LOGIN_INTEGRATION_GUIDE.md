# Casdoor 登录对接完整指南

## 📋 概述

Casdoor 是一个开源的认证/授权平台，支持 OAuth 2.0 和 OpenID Connect。本指南说明如何将 Casdoor 与 OpenIM Chat 集成，实现单点登录（SSO）。

### 支持的登录方式

1. **OAuth 2.0 授权码流程** - 标准 OAuth，需要重定向到浏览器
2. **资源所有者密码凭证** - 用户名/密码直接登录（适合移动应用）
3. **Webhook 同步** - Casdoor 主动通知用户变更

---

## 🔧 第一步：配置 Casdoor 服务器

### 1. 部署 Casdoor 实例

```bash
# Docker 方式（最快）
docker run -d \
  -p 8000:8000 \
  -e "db_driver=sqlite" \
  -e "db_data_source=:memory:" \
  casbin/casdoor:latest

# 或从源码编译
git clone https://github.com/casdoor/casdoor.git
cd casdoor
go run main.go
# 访问 http://localhost:8000
```

### 2. 在 Casdoor 中创建应用

访问 Casdoor 管理面板：http://localhost:8000/admin

**创建步骤**:

1. 左侧菜单 → **应用** → **添加应用**
2. 填写表单：
   ```
   名称: OpenIM Chat
   显示名: OpenIM Chat App
   Logo: [可选]
   主页: https://your-app-domain
   ```

3. 添加 **重定向 URI**（关键！）
   ```
   http://localhost:8888/account/casdoor/callback
   https://your-app-domain/account/casdoor/callback
   ```
   > 注：`8888` 是默认的 Chat API 端口，根据实际配置调整

4. 选择 **提供商** 
   - 默认使用 Casdoor 内置用户数据库
   - 或配置 Google/GitHub/微信等第三方

5. 点击 **保存**

6. 记录以下信息：
   ```
   Client ID:     (应用 ID)
   Client Secret: (应用密钥)
   ```

### 3. 配置登出 URL（可选）

```
Post Logout Redirect URI: https://your-app-domain/login
```

---

## ⚙️ 第二步：配置 OpenIM Chat

### 编辑配置文件

编辑 `config/chat-rpc-chat.yml`:

```yaml
# ========================================
# Casdoor SSO Configuration
# ========================================
casdoor:
  # 启用/禁用 Casdoor
  enabled: true
  
  # Casdoor 服务器地址
  endpoint: "http://your-casdoor-server:8000"
  
  # 从 Casdoor 应用获取
  clientId: "your-app-client-id"
  clientSecret: "your-app-client-secret"
  
  # 用于验证 Webhook 签名（Casdoor → Chat 通知）
  syncSecret: "your-webhook-secret"
  
  # 性能优化
  redisCache: true         # 启用 Redis 存储 OAuth state
  enableMetrics: true      # 启用性能监控
  enableTracing: false     # 暂不启用分布式追踪

# ========================================
# Redis Configuration (如果启用 redisCache)
# ========================================
redis:
  host: "127.0.0.1"
  port: 6379
  password: ""
  db: 0
```

### 环境变量方式（Docker/K8s）

```bash
# 优先级：环境变量 > 配置文件

export CHATENV_CHAT_CASDOOR_ENABLED=true
export CHATENV_CHAT_CASDOOR_ENDPOINT=http://casdoor-service:8000
export CHATENV_CHAT_CASDOOR_CLIENT_ID=your-app-id
export CHATENV_CHAT_CASDOOR_CLIENT_SECRET=your-app-secret
export CHATENV_CHAT_CASDOOR_SYNC_SECRET=your-webhook-secret
```

### 启动服务

```bash
mage start    # 启动所有服务
mage check    # 验证启动状态
```

---

## 🌐 第三步：前端集成

### 登录流程图

```
┌─ 用户 ─────────────────────────────────────────────────────────┐
│                                                                  │
│  1. 点击"SSO 登录"                                              │
│     ↓                                                            │
│  2. 前端调用：POST /account/casdoor/login_url                  │
│     请求体: {                                                   │
│       "client": "web",  // 客户端类型                          │
│       "platform": 1,    // iOS=1, Android=2, Web=3, macOS=4   │
│       "redirectURI": "https://your-app/auth/callback"          │
│     }                                                            │
│     ↓                                                            │
│  3. 服务器返回登录 URL + State                                  │
│     响应: {                                                      │
│       "url": "http://casdoor:8000/login/oauth/authorize?...",│
│       "state": "random-state-string"                           │
│     }                                                            │
│     ↓                                                            │
│  4. 前端重定向到 Casdoor                                        │
│     window.location = response.url                             │
│     ↓                                                            │
│  5. 用户在 Casdoor 登录（输入用户名密码）                       │
│     ↓                                                            │
│  6. Casdoor 重定向回应用                                        │
│     GET https://your-app/auth/callback?code=xxx&state=yyy     │
│     ↓                                                            │
│  7. 前端提取 code 和 state                                      │
│     ↓                                                            │
│  8. 前端调用：POST /account/casdoor/callback                    │
│     请求体: {                                                   │
│       "code": "authorization-code",                            │
│       "state": "same-state-from-step-3",                       │
│       "redirectURI": "https://your-app/auth/callback",         │
│       "platform": 1,                                           │
│       "deviceID": "device-identifier"                          │
│     }                                                            │
│     ↓                                                            │
│  9. 服务器验证 code 和 state，获取用户信息                      │
│     ↓                                                            │
│ 10. 服务器创建/更新用户，生成 Token                            │
│     ↓                                                            │
│ 11. 返回登录响应                                                │
│     响应: {                                                      │
│       "userID": "user-123",                                     │
│       "chatToken": "jwt-token",                                │
│       "imToken": "im-sdk-token"                                │
│     }                                                            │
│     ↓                                                            │
│ 12. 前端保存 Token，跳转到主页                                  │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 前端实现示例（JavaScript/React）

```javascript
// 1. 登录 URL 获取
async function getCasdoorLoginURL() {
  const response = await fetch('/account/casdoor/login_url', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      client: 'web',
      platform: 3,  // Web
      redirectURI: `${window.location.origin}/auth/callback`
    })
  });
  
  const data = await response.json();
  console.log('Login URL:', data.data.url);
  console.log('State:', data.data.state);
  
  // 保存 state 到 sessionStorage（用于验证回调）
  sessionStorage.setItem('casdoor_state', data.data.state);
  
  // 重定向到 Casdoor 登录
  window.location.href = data.data.url;
}

// 2. 处理回调（在 /auth/callback 页面）
async function handleCasdoorCallback() {
  // 从 URL 中提取 code 和 state
  const params = new URLSearchParams(window.location.search);
  const code = params.get('code');
  const state = params.get('state');
  
  // 验证 state（防止 CSRF 攻击）
  const savedState = sessionStorage.getItem('casdoor_state');
  if (state !== savedState) {
    console.error('State mismatch! Possible CSRF attack');
    window.location.href = '/login';
    return;
  }
  
  // 调用服务器回调 API
  try {
    const response = await fetch('/account/casdoor/callback', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        code: code,
        state: state,
        redirectURI: window.location.origin + '/auth/callback',
        platform: 3,
        deviceID: generateDeviceID()
      })
    });
    
    const result = await response.json();
    
    if (!response.ok) {
      console.error('Login failed:', result.errMsg);
      window.location.href = '/login?error=' + result.errMsg;
      return;
    }
    
    // 保存 Token
    localStorage.setItem('userID', result.data.userID);
    localStorage.setItem('chatToken', result.data.chatToken);
    localStorage.setItem('imToken', result.data.imToken);
    
    // 清理 state
    sessionStorage.removeItem('casdoor_state');
    
    // 重定向到主页
    window.location.href = '/home';
    
  } catch (error) {
    console.error('Callback error:', error);
    window.location.href = '/login?error=callback_failed';
  }
}

// 3. 登出
async function logout() {
  const redirectURI = window.location.origin + '/login';
  
  const response = await fetch('/account/casdoor/logout', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + localStorage.getItem('chatToken')
    },
    body: JSON.stringify({
      redirectURI: redirectURI
    })
  });
  
  const result = await response.json();
  
  // 清理本地存储
  localStorage.removeItem('userID');
  localStorage.removeItem('chatToken');
  localStorage.removeItem('imToken');
  
  // 如果 Casdoor 返回了登出 URL，重定向
  if (result.data?.logoutURL) {
    window.location.href = result.data.logoutURL;
  } else {
    window.location.href = '/login';
  }
}

// 辅助函数
function generateDeviceID() {
  // 生成或获取设备 ID
  let deviceID = localStorage.getItem('deviceID');
  if (!deviceID) {
    deviceID = 'device_' + Math.random().toString(36).substr(2, 9);
    localStorage.setItem('deviceID', deviceID);
  }
  return deviceID;
}
```

### HTML 模板

```html
<!DOCTYPE html>
<html>
<head>
  <title>OpenIM Chat - 登录</title>
</head>
<body>
  <div class="login-container">
    <h1>OpenIM Chat</h1>
    
    <!-- SSO 登录按钮 -->
    <button onclick="getCasdoorLoginURL()" class="btn btn-primary">
      使用 Casdoor 登录
    </button>
    
    <!-- 分隔线 -->
    <div class="divider">或</div>
    
    <!-- 用户名/密码登录（备选方案）-->
    <form id="passwordForm">
      <input type="text" placeholder="用户名" id="username" required />
      <input type="password" placeholder="密码" id="password" required />
      <button type="button" onclick="loginWithPassword()" class="btn btn-secondary">
        密码登录
      </button>
    </form>
  </div>
  
  <script src="app.js"></script>
</body>
</html>
```

---

## 🔐 方式二：密码登录（可选）

### API 端点：`POST /account/casdoor/password`

直接使用用户名密码登录，无需浏览器重定向。

**请求体**:
```json
{
  "username": "casdoor_user",
  "password": "password123",
  "platform": 3,
  "deviceID": "device_123"
}
```

**响应**:
```json
{
  "code": 0,
  "data": {
    "userID": "user_123",
    "chatToken": "jwt_token_xxx",
    "imToken": "im_token_yyy"
  }
}
```

**前端示例**:
```javascript
async function loginWithPassword() {
  const username = document.getElementById('username').value;
  const password = document.getElementById('password').value;
  
  const response = await fetch('/account/casdoor/password', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: username,
      password: password,
      platform: 3,
      deviceID: generateDeviceID()
    })
  });
  
  const result = await response.json();
  
  if (!response.ok) {
    alert('登录失败: ' + result.errMsg);
    return;
  }
  
  // 保存 Token
  localStorage.setItem('userID', result.data.userID);
  localStorage.setItem('chatToken', result.data.chatToken);
  localStorage.setItem('imToken', result.data.imToken);
  
  // 跳转到主页
  window.location.href = '/home';
}
```

---

## 🔔 第四步：Webhook 同步（可选）

### 场景

Casdoor 中用户信息变更时（头像、昵称、禁用状态等），自动通知 OpenIM Chat 更新用户信息。

### 配置 Casdoor Webhook

在 Casdoor 管理面板：

1. **应用** → 选择你的应用
2. **Webhook** → **添加 Webhook**
3. 填写：
   ```
   URL: https://your-chat-api:8888/account/casdoor/sync
   事件: user.updated, user.deleted
   请求头:
     Authorization: Bearer your-webhook-secret
   ```

### 接收 Webhook

**API 端点**：`POST /account/casdoor/sync`

**请求体** (来自 Casdoor):
```json
{
  "profile": {
    "userId": "casdoor_user_id",
    "name": "John Doe",
    "email": "john@example.com",
    "phone": "+86 1234567890",
    "nickname": "john_doe",
    "faceUrl": "https://casdoor.example.com/avatar.jpg",
    "disabled": false
  },
  "secret": "your-webhook-secret"
}
```

**服务器处理**:
```go
// internal/api/chat/casdoor.go
func (o *Api) CasdoorSync(c *gin.Context) {
  // 1. 验证 secret
  // 2. 查找/创建用户绑定
  // 3. 更新用户信息到 OpenIM
  // 4. 返回成功响应
}
```

---

## 📊 API 端点总览

| 端点 | 方法 | 说明 | 认证 |
|------|------|------|------|
| `/account/casdoor/login_url` | POST | 获取 Casdoor 登录 URL | ❌ |
| `/account/casdoor/callback` | POST | 处理 OAuth 回调 | ❌ |
| `/account/casdoor/password` | POST | 用户名密码登录 | ❌ |
| `/account/casdoor/logout` | POST | 登出 | ✅ Token |
| `/account/casdoor/sync` | POST | Webhook 同步 | ✅ Secret |

---

## 🔍 用户流映射

### 内部用户 ID 映射

当用户首次通过 Casdoor 登录时：

```
Casdoor User ID (e.g., "casdoor_123")
         ↓
OpenIM User ID (generated as "user_<hash>")
         ↓
存储在 MongoDB:
  collection: casdoor_bindings
  doc: {
    "_id": ObjectID,
    "casdoor_user_id": "casdoor_123",
    "openim_user_id": "user_abc123",
    "created_at": timestamp,
    "updated_at": timestamp,
    "sync_metadata": {...}
  }
```

### 后续登录流程

```
用户用 Casdoor 登录
  ↓
查询 casdoor_bindings 表
  ↓
  ├─ 找到绑定？ → 使用已有 OpenIM User ID
  │
  └─ 未找到？ → 创建新 OpenIM 用户 → 创建绑定关系
```

---

## 🐛 常见问题排查

### 问题 1：重定向 URI 不匹配错误

**症状**: 
```
error: redirect_uri_mismatch
```

**解决**:
1. 检查 Casdoor 应用配置中的"重定向 URI"列表
2. 确保前端传送的 `redirectURI` 完全匹配配置
3. 注意 `http://` 和 `https://` 的区别
4. 检查端口号是否正确

### 问题 2：State 验证失败

**症状**:
```
error: state validation failed
```

**解决**:
1. 检查 `enableRedisCache` 是否启用（多实例部署需要 Redis）
2. 检查 Redis 连接是否正常
3. 确认 State TTL（10分钟内必须完成流程）
4. 浏览器清空 Cookie/SessionStorage

### 问题 3：找不到用户信息

**症状**:
```
error: failed to get user profile from access token
```

**解决**:
1. 检查 Casdoor 中的应用范围是否包括 `profile email`
2. 检查 Casdoor 用户是否完整填写了必要字段
3. 检查 Casdoor 服务器是否可达
4. 查看日志：`log grep "casdoor" | grep error`

### 问题 4：无法创建用户

**症状**:
```
error: failed to create user in OpenIM
```

**解决**:
1. 检查 OpenIM 服务器是否启动：`mage check`
2. 检查 OpenIM 用户数量是否达到限制
3. 检查昵称/头像 URL 是否有效
4. 检查数据库连接是否正常

### 问题 5：多实例部署 State 丢失

**症状**: 登录时收到"state expired"或"state not found"

**解决**:
1. 启用 Redis：`redisCache: true`
2. 检查 Redis 服务是否运行
3. 检查 Redis 连接配置
4. 查看日志中的 Redis 错误

---

## 📈 生产环境检查清单

- [ ] Casdoor 服务器已部署并可访问
- [ ] 应用已在 Casdoor 创建，记录了 Client ID 和 Secret
- [ ] 重定向 URI 已在 Casdoor 中配置
- [ ] `config/chat-rpc-chat.yml` 已正确配置
- [ ] Redis 已部署（多实例环境）
- [ ] OpenIM 服务器已启动
- [ ] Chat API 服务已启动
- [ ] HTTPS 已配置（如需要）
- [ ] DNS 已指向正确的服务器
- [ ] 防火墙已开放必要端口
- [ ] 日志级别已设置为正确的值
- [ ] 监控告警已配置（性能指标）
- [ ] 备份和恢复流程已测试
- [ ] 安全审计已完成

---

## 🔗 相关文档

- [Casdoor 官方文档](https://casdoor.org/)
- [OAuth 2.0 规范](https://tools.ietf.org/html/rfc6749)
- [OpenID Connect](https://openid.net/connect/)
- [OpenIM 用户管理](./IMPROVEMENTS_DETAIL.md)

---

## 📞 支持

遇到问题？检查以下资源：

1. **服务器日志**
   ```bash
   docker logs chat-api  # Docker 环境
   tail -f logs/chat.log  # 本地环境
   ```

2. **健康检查**
   ```bash
   curl http://localhost:8080/api/health | jq .
   ```

3. **性能指标**
   ```bash
   curl http://localhost:8080/api/metrics/casdoor | jq .
   ```

---

**最后更新**: 2026-11-05  
**版本**: 1.0  
**维护者**: OpenIM 团队
