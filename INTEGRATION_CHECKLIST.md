# ✅ 功能集成快速清单

## 🎯 目标
将 `local-feature` 分支中的 **Casdoor SSO** 和 **RTC** 功能集成到 `main` 分支

## 📊 当前进度

```
已完成: ████████░░ 80%
```

| 任务 | 状态 | 进度 |
|------|------|------|
| 代码文件复制 | ✅ 完成 | 100% |
| API 实现 | ✅ 完成 | 100% |
| RPC 实现 | ✅ 完成 | 100% |
| 数据库模型 | ✅ 完成 | 100% |
| 路由注册 | ✅ 完成 | 100% |
| 配置初始化 | ⏳ 进行中 | 50% |
| 协议定义 | ⏳ 待开始 | 0% |
| 编译验证 | ❌ 失败 | - |
| 单元测试 | ❌ 待开始 | 0% |
| 集成测试 | ❌ 待开始 | 0% |

---

## 🔧 必完成任务 (P0)

### Task 1: 更新 Proto 定义
**文件**: `pkg/protocol/chat/chat.proto`
**预计时间**: 30 分钟
**难度**: ⭐

```protobuf
// 在 chat.proto 中添加
syntax = "proto3";

package chat;

// Casdoor 用户信息
message CasdoorProfile {
    string user_id = 1;
    string name = 2;
    string email = 3;
    string phone = 4;
    string nickname = 5;
    string face_url = 6;
    bool disabled = 7;
}

// 登录请求
message CasdoorLoginReq {
    CasdoorProfile profile = 1;
    int32 platform = 2;
    string device_id = 3;
}

// 登录响应
message CasdoorLoginResp {
    string user_id = 1;
    string chat_token = 2;
    string im_token = 3;
}

// 同步请求
message CasdoorSyncReq {
    CasdoorProfile profile = 1;
}

// 同步响应
message CasdoorSyncResp {
    bool success = 1;
    string message = 2;
}

// 在 ChatService 中添加
service Chat {
    // 现有方法...
    
    // 新增方法
    rpc CasdoorLogin(CasdoorLoginReq) returns(CasdoorLoginResp);
    rpc CasdoorSync(CasdoorSyncReq) returns(CasdoorSyncResp);
}
```

**验证命令**:
```bash
cd pkg/protocol/chat
protoc --go_out=. --go-grpc_out=. chat.proto
```

---

### Task 2: 扩展 Config 结构体
**文件**: `pkg/common/config/config.go`
**预计时间**: 20 分钟
**难度**: ⭐

```go
// 在 Chat 配置中添加
type Chat struct {
    // 现有配置...
    
    // Casdoor 配置
    Casdoor CasdoorConfig `mapstructure:"casdoor"`
    
    // LiveKit 配置
    LiveKit LiveKitConfig `mapstructure:"liveKit"`
}

type CasdoorConfig struct {
    Enabled      bool   `mapstructure:"enabled"`
    Endpoint     string `mapstructure:"endpoint"`
    ClientID     string `mapstructure:"clientId"`
    ClientSecret string `mapstructure:"clientSecret"`
    SyncSecret   string `mapstructure:"syncSecret"`
}

type LiveKitConfig struct {
    URL              string `mapstructure:"url"`
    Key              string `mapstructure:"key"`
    Secret           string `mapstructure:"secret"`
    IngressRTMPBase  string `mapstructure:"ingressRTMPBase"`
    EgressRTMPBase   string `mapstructure:"egressRTMPBase"`
    PlaybackRTMPBase string `mapstructure:"playbackRTMPBase"`
}
```

**配置文件示例** (`config/chat-rpc-chat.yml`):
```yaml
casdoor:
  enabled: false  # 默认关闭
  endpoint: "https://casdoor.example.com"
  clientId: "your-client-id"
  clientSecret: "your-client-secret"
  syncSecret: "your-sync-secret"

liveKit:
  url: "ws://livekit:7880"
  key: "api-key"
  secret: "api-secret"
  ingressRTMPBase: "rtmp://livekit:1935/live"
  egressRTMPBase: "rtmp://srs:19350/live"
  playbackRTMPBase: "rtmp://srs:19350/live"
```

---

### Task 3: 扩展数据库接口
**文件**: `pkg/common/db/database/chat.go`
**预计时间**: 30 分钟
**难度**: ⭐⭐

```go
// 添加到 ChatDatabaseInterface
type ChatDatabaseInterface interface {
    // 现有方法...
    
    // Casdoor 绑定方法
    TakeCasdoorBindingByCasdoorUserID(ctx context.Context, userID string) (*chatdb.CasdoorBinding, error)
    TakeCasdoorBindingByOpenIMUserID(ctx context.Context, userID string) (*chatdb.CasdoorBinding, error)
    CreateCasdoorBinding(ctx context.Context, binding *chatdb.CasdoorBinding) error
    UpdateCasdoorBinding(ctx context.Context, binding *chatdb.CasdoorBinding) error
    DeleteCasdoorBinding(ctx context.Context, casdoorUserID string) error
}

// 在 MongoDB 实现中添加方法 (pkg/common/db/database/mongodb/chat.go)
func (m *mongoDb) TakeCasdoorBindingByCasdoorUserID(ctx context.Context, userID string) (*chatdb.CasdoorBinding, error) {
    binding := &chatdb.CasdoorBinding{}
    err := m.collection(tableNameCasdoorBinding).FindOne(ctx, bson.M{
        "casdoor_user_id": userID,
    }).Decode(binding)
    if err == mongo.ErrNoDocuments {
        return nil, nil
    }
    return binding, err
}

func (m *mongoDb) CreateCasdoorBinding(ctx context.Context, binding *chatdb.CasdoorBinding) error {
    _, err := m.collection(tableNameCasdoorBinding).InsertOne(ctx, binding)
    return err
}

func (m *mongoDb) UpdateCasdoorBinding(ctx context.Context, binding *chatdb.CasdoorBinding) error {
    _, err := m.collection(tableNameCasdoorBinding).UpdateOne(ctx,
        bson.M{"casdoor_user_id": binding.CasdoorUserID},
        bson.M{"$set": binding},
    )
    return err
}
```

---

### Task 4: 生成 Proto 代码
**文件**: `pkg/protocol/chat/chat*.pb.go`
**预计时间**: 5 分钟
**难度**: ⭐

```bash
cd /Users/wangpc/worksapce/zkys/im/chat

# 方法1: 使用 make
make gen-api

# 方法2: 直接使用 protoc
cd pkg/protocol/chat
protoc --go_out=. --go-grpc_out=. chat.proto

# 验证生成结果
ls -la *pb*.go
```

---

### Task 5: 编译验证
**预计时间**: 5 分钟
**难度**: ⭐

```bash
cd /Users/wangpc/worksapce/zkys/im/chat

# 清理依赖
go mod tidy

# 编译检查
go build -o /tmp/chat-api ./cmd/api/chat-api

# 如果成功，输出
echo "✅ Build successful!"
```

---

## 🧪 测试任务 (P1)

### Task 6: 单元测试
**预计时间**: 2 小时
**难度**: ⭐⭐

```bash
# 运行所有 Casdoor 相关测试
go test ./internal/api/chat -v -run "Casdoor"
go test ./internal/rpc/chat -v -run "Casdoor"
go test ./pkg/common/db -v -run "Casdoor"

# 输出覆盖率
go test -cover ./internal/api/chat
go test -cover ./internal/rpc/chat
```

### Task 7: 集成测试
**预计时间**: 2 小时
**难度**: ⭐⭐⭐

```bash
# 启动依赖服务
docker-compose up -d mongodb redis

# 运行集成测试
go test ./test/integration -v

# 清理
docker-compose down
```

---

## 📝 可选任务 (P2)

- [ ] 添加性能测试
- [ ] 添加负载测试
- [ ] 安全扫描 (gosec)
- [ ] 代码质量检查 (golangci-lint)

---

## 🚀 部署前检查

- [ ] 所有编译错误已解决
- [ ] 单元测试通过率 > 90%
- [ ] 无 Critical 级别的安全扫描结果
- [ ] 代码覆盖率 > 70%
- [ ] 文档已更新
- [ ] 配置示例已提供
- [ ] 变更日志已记录

---

## 📞 快速参考

### 常见错误及解决

**错误**: `undefined: chat.CasdoorProfile`
**原因**: Proto 定义不完整
**解决**: 完成 Task 1 并运行 `make gen-api`

**错误**: `config.Chat has no field Casdoor`
**原因**: 配置结构体未扩展
**解决**: 完成 Task 2

**错误**: `database.ChatDatabaseInterface has no method TakeCasdoorBindingByCasdoorUserID`
**原因**: 数据库接口未扩展
**解决**: 完成 Task 3

### 有用的命令

```bash
# 查看当前分支状态
git status

# 查看分支差异
git diff main feat/integrate-casdoor-rtc

# 运行所有测试
go test ./... -v

# 代码质量检查
golangci-lint run -c .golangci.yml

# 安全扫描
gosec ./...
```

---

## 📊 预期时间线

```
总时间: 4-5 天

Day 1: 协议定义 + 配置 + 数据库 (1.5 小时)
  └─ P0 任务完成，编译通过

Day 2: 单元测试 (2 小时)
  └─ 代码逻辑验证

Day 3: 集成测试 (2 小时)
  └─ 端到端功能验证

Day 4: 性能优化 + 文档 (2 小时)
  └─ 代码审查，最佳实践应用

Day 5: 最终检查 + 部署准备 (1 小时)
  └─ 生产就绪检查清单
```

---

## ✅ 完成标准

集成认为"完成"需要满足以下条件：

- [x] 所有 P0 任务完成
- [x] 编译通过，无警告
- [x] 单元测试覆盖率 > 70%
- [x] 集成测试通过
- [x] API 文档完整
- [x] 配置示例可用
- [x] 变更日志已记录
- [x] 代码审查通过

---

**更新时间**: 2026-11-05  
**分支**: `feat/integrate-casdoor-rtc`  
**提交**: `8d888d4`  
**状态**: 80% 完成，待 Task 1-5
