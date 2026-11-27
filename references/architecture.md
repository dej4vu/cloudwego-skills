# 架构与目录

单仓多服务、网关与 RPC 的职责边界、每个文件放哪。

## 仓库形状 `[主]`

```text
<repo>/
├── app/
│   ├── apigw/            Hertz，用户端入口
│   ├── admingw/          Hertz，管理端入口
│   ├── queue/            Kafka 消费者（集中消费所有 topic）
│   └── {service}/        Kitex RPC 服务
├── idl/
│   ├── rpc/{svc}/        领域 RPC 契约
│   ├── apigw/            用户端 HTTP 契约（含 api.proto）
│   ├── admingw/          管理端 HTTP 契约
│   └── errno/            错误码
├── rpc_gen/              Kitex 生成物 + cwgo 客户端包装（禁止编辑）
├── common/               RPC 侧公共库（cachex、gormx、gwjson、mq、queue/kq、redislock、ratelimit …）— 禁止引入 Hertz
├── hcommon/              网关公共库（pkg/responsex、pkg/mw、pkg/confx）— 仅 apigw / admingw 使用
└── templates/cwgo/       cwgo 自定义模板
```

`go.work` 把 `app/*`、`rpc_gen`、`common`、`hcommon` 串成工作区；每个目录是独立 module，`app/{svc}/go.mod` 通过 `replace` 指向本地模块。

单个独立服务不需要这套布局，只需保持下文"服务内目录"的形状。

`common` 与 `hcommon` 的边界只有一条：`common` 服务于 Kitex RPC，不引入 Hertz 依赖；凡是 import 了 `github.com/cloudwego/hertz` 的公共代码放 `hcommon`。拿不准时问"这段代码需不需要 `app.RequestContext`"，需要就放 `hcommon`。

## 服务内目录 `[主]`

```text
app/{service}/
├── main.go                        启动顺序：kitexInit → dal.Init → rpc.InitClient → service.InitDeps
├── handler.go                     🚫 生成
├── biz/
│   ├── service/
│   │   ├── init.go                依赖装配（dependencies / d() / InitDeps / ShutdownDeps）
│   │   └── {method}.go            ✅ 业务编排，只在这里
│   ├── dao/{entity}_dao.go        ✅ 数据访问，一文件一 DAO
│   ├── model/{entity}.go          ✅ GORM 模型，不在代码里定义索引
│   ├── pack/{entity}.go           ✅ model ↔ RPC 类型转换
│   ├── producer/                  ✅ Kafka producer
│   └── dal/                       ✅ 存储客户端初始化
├── infra/rpc/client.go            ✅ 下游 RPC client
└── conf/                          ✅ 配置
```

依赖方向单向：`handler → service → dao → pack`。service 不被 dao 反向引用，pack 不依赖 service。

网关的结构对应：

```text
app/apigw/
├── main.go                        rpc.InitClient → server.New(WithBindConfig) → 中间件 → 路由
├── router.go                      customizedRegister：非 IDL 路由（支付回调、ping）
├── biz/handler/{pkg}/             🚫 生成，流式桥接除外
├── biz/router/{pkg}/              🚫 生成；middleware.go 是用户扩展点
├── biz/service/{method}.go        ✅ 认证后的编排 + pack + 调 RPC
├── biz/mw/auth.go                 ✅ 认证中间件（只保留与身份模型绑定的部分）
├── biz/pack/                      ✅ 网关类型 ↔ RPC 类型
├── biz/service/support.go         ✅ clientInfo / requestID 等网关级辅助
├── hertz_gen/                     🚫 生成
└── infra/rpc/client.go            ✅
```

响应信封、请求 ID、访问日志、客户端 IP、CORS 都在 `hcommon/`，网关里不再有 `biz/utils/`。配置加载用 `hcommon/pkg/confx`（`GO_ENV` → `conf/<env>/conf.yaml`）。

## 职责边界 `[主]`

| 角色 | 做 | 不做 |
|---|---|---|
| `apigw` / `admingw` | 解析 token、校验登录态、路由、pack、调 RPC | 直连数据库；持有领域事实；业务规则 |
| 领域 RPC 服务 | 拥有表、实现领域规则、调下游 RPC | 接收或重复校验用户 token |
| `queue` | 反序列化、调领域 RPC、重试与 DLQ | 业务逻辑；长耗时执行（交给专用 worker） |

每张表只有一个 owner service。整个项目每个环境**共用一个数据库**（不按服务拆库），但共用库不等于可以跨服务读写彼此的表——需要别人的数据就发 RPC。服务间通信只走 RPC。

`queue` 的 handler 只做"反序列化 → 调领域 RPC → 返回 error"，业务逻辑落在领域服务里。仓库目前没有独立的定时任务服务，定时与对账需求出现时再单独设计，不要把这类逻辑塞进 `queue` 或网关。

## 服务拆分判据 `[主]`

- 实时链路要内聚：有状态的长链路留在同一个服务，不为"微服务"而外拆。
- 非实时必需的工作走异步：对账、摘要、用量结算、内容审核、资产生成。
- 长耗时任务（分钟级生成、转码）只投递任务消息，执行落在专用 worker，不阻塞 `queue` 消费。
- 新增 RPC 的部署顺序：先部署 callee，再部署 caller。

## 开发纪律 `[主]`

非 trivial 变更都要守：

1. **先规划再动手。** 新服务、新接口、跨服务调用、数据模型变更，先给出简要计划（改哪些文件、IDL 是否变更、部署顺序影响），确认边界后再写。
2. **Simplicity First。** 只实现当前任务要求的功能，不做投机性抽象，不"顺便优化"无关代码。
3. **Surgical Changes。** 只改与任务直接相关的文件。
4. **Goal-Driven Execution。** 每一步产出可验证：编译通过、测试通过、生成物一致。
5. **每完成一个逻辑单元提交一次。**

## 数据保护取舍 `[主]`

初创项目遵循"非必要不加密/不哈希"：默认使用可排查的明文字段，只有密码、验证码、手机号、身份证等高敏或需要不可逆凭证的字段才加密或哈希。IP、设备 ID、运营上下文默认明文，靠短保留周期和访问控制控风险。新增加密/哈希字段必须在设计文档里说明必要性、可逆性、排查方式和密钥管理成本。

这条与"日志不打印用户正文"不矛盾：前者是存储层的可排查性取舍，后者是日志不得成为内容的第二个副本。

## 依赖装配 `[主]`

业务依赖集中在 `biz/service/init.go`，一次装配，fail-fast：

```go
type dependencies struct {
    inventoryClient inventoryClient // 窄接口，只声明本服务用到的方法
    orderStore    orderStore
}

var deps *dependencies

// d 返回已装配依赖；未初始化时快速失败。
func d() *dependencies {
    if deps == nil {
        panic("service deps not initialized: call service.InitDeps before serving requests")
    }
    return deps
}

// InitDeps 在 main 中服务启动前调用一次；重复调用或装配失败直接 panic。
func InitDeps() { /* ... */ }

// ShutdownDeps 停止后台 worker、关闭连接；可重复调用。
func ShutdownDeps() { /* ... */ }
```

约定：

- `{method}.go` 只写编排，通过 `d()` 取依赖，不在方法内临时初始化客户端。
- 下游 client 用**窄接口**承载，只列出本服务调用的方法，便于替换和测试。
- 依赖很少的服务可以用包级变量代替 `dependencies` 聚合，但仍集中在 `init.go`。
- 网关是例外：RPC client 统一放 `infra/rpc/client.go`，由 `main` 调 `InitClient()`。

详见 [kitex-service.md](kitex-service.md)。

## 公共演进：网关公共库 `[主]`

两个仓库都有独立 module `hcommon/`，`common/` 保持零 Hertz 依赖——这条边界是共有的。差别在网关自己还剩多少公共代码：

| | 对照库 `hcommon/` | 网关里剩下的 |
|---|---|---|
| 对照库 | `accesslogx`、`corsx`、`encodingx`、`errorx`、`signx` | `biz/utils/{resp,reqctx,scene}.go`：响应信封仍在网关内 |
| 主参考库 | `responsex`、`mw`、`confx` | 无 `biz/utils/`；`biz/mw/auth.go` 只剩身份模型相关的部分 |

主参考库把响应信封也抽了出去，因为信封的 wire 契约（protojson、字符串 timestamp）和绑定配置是 `apigw` / `admingw` 必须完全一致的，留在各自网关里迟早漂移。新仓库从一开始就按主参考库的划分放代码。

## 与对照库的差异

对照库没有集中装配：依赖在各个 `{method}.go` 里各自构造，DAO 也直接在 service 内 `New`。服务小时差别不大，服务变大后每加一个下游要改多个文件，且无法在启动时发现装配错误。主参考库的集中装配是为解决这个问题演进出来的，新代码一律按主参考库。
