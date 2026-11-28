# 兼容性

验证过的版本组合、两条生成路径的差异、升级时的注意事项。

## 验证矩阵

| 组件 | 版本 | 说明 |
|---|---|---|
| cwgo | `v0.1.3`（fork） | 嵌入 Kitex `v0.16.3` + hz `v0.9.7` + thriftgo `v0.4.5` |
| Kitex runtime | `v0.16.4` | `go mod tidy` 解析结果；生成器 `v0.16.3` 与之互通 |
| Hertz runtime | `v0.10.6` | |
| hz | `v0.9.7` | cwgo 内嵌版本；本机独立安装应保持一致 |
| Go | `1.25` / `1.27` | cwgo 自身用 1.25 构建；生成项目用 1.27 |

这套组合下 fixture 全部通过 `go build` 与 `go test`。升级任一项都要同步更新本表并重跑 fixture。

## 生成路径的差异

| | cwgo fork | 原生 hz / kitex |
|---|---|---|
| 网关 handler | 委托 `service.NewX(ctx,c).Run(&req)` + `utils.Send*Response`（两个失败分支都是 `StatusOK`） | `hz new` / `hz update` 追加的方法直接 `c.JSON`，不委托 service |
| 网关 service 桩 | 生成空桩 `biz/service/{method}.go` | 不生成，手写 |
| `biz/utils/resp.go` | 生成桩 | 不生成 |
| `--unset_omitempty` | 必须经 `--pass` 透传 | 直接给 |
| 占位 pb 包 | 真实生成到 `hertz_gen/`（需清理，否则与其他链路的真实 genproto 重复注册） | `--no_recurse` 下不生成，留下悬空 blank import |
| RPC client 包装 | `rpc_gen/rpc/{svc}` 带默认地址 | 无 |
| proto 搜索路径参数 | `--proto_search_path` / `-I` | `--proto_path` / `-I` |
| 维护状态 | fork 持续维护 | 官方 |

cwgo 上游已归档并推荐新用户用原生工具。两个参考库都基于 fork 继续维护 Kitex 侧（`cwgo client` / `cwgo server --type RPC` + `templates/cwgo/`）。网关侧两者分道：对照库用 `cwgo server --type HTTP`，主参考库用原生 `hz update` 维护一份 cwgo 形状的 handler。不要在一个仓库里混用。

### 响应信封与 JSON 形态

| | 对照库 | 主参考库 |
|---|---|---|
| 响应编码 | `c.JSON` + `encoding/json` | `pbjson` + protojson（`EmitUnpopulated`） |
| `int64` 字段 | 数字（JS 丢精度） | 字符串 |
| 零值字段 | 取决于 `omitempty` | 保留 |
| `timestamp` | 数字 | 字符串 |
| 请求绑定 | Hertz 默认解码器 | protojson（`server.WithBindConfig`） |
| 错误分类入口 | `SendErrResponse` 一个，内部按类型与文案匹配 | `SendErrResponse` / `SendBindErrorResponse` 两个，按分支选 |

这些是对前端的契约。同一个网关不要中途换形态；新网关按主参考库。

## 已知的版本坑

### hz v0.9.7 的 `--protoc M` 参数不生效

原生 hz 用 `--no_recurse` 时，无法通过 `--protoc M` 参数把 `google.api` 的 import 映射到官方包，生成的 blank import 指向本地占位目录。两种修法见 [toolchain-and-generation.md](toolchain-and-generation.md)：去掉 `--no_recurse`，或手改那一行。主参考库的 `make gen-gw` 选的是后者（apigw 改指官方包，admingw 直接删行）。

### PATH 里的 `kitex` 与 cwgo 内置版本不一致

本机 `kitex` 若是 `v0.14.1`，用它生成 cwgo 仓库的 `kitex_gen` 会产生大量无关 diff（版本标记、生成代码差异）。cwgo 仓库一律用 `cwgo` 生成。

### 流式 handler 骨架的 `context.Background()`

Kitex `v0.16.3` 生成的流式 handler 不透传请求 ctx。这是生成器行为，升级前先确认新版是否已修复；未修复则继续遵守 `s.ctx = stream.Context()`。

### hz 按方法名增量合并 handler

`hz update` 只追加缺失方法，不覆盖已有方法体，也不清理 IDL 里已删除的方法。追加的新方法是 hz 原生形状（`c.JSON`，不委托 service）。升级 hz 后重新验证这两点，它们决定了手改的 handler 和流式桥接是否安全。

### 响应函数签名在两个网关间必须一致

`apigw` 与 `admingw` 共用 `hcommon/pkg/responsex`。给 `responsex` 加参数或改签名时两个网关同时受影响，改完两个都要编译。

### 网关配置包不能抽到 `hcommon`

`hz` 生成的模板按 `<module>/conf` 引用配置包。主参考库曾把它抽成 `hcommon/pkg/confx`，`make gen-gw` 随即失败，又撤回各网关本地。网关配置包一律留在 `app/{gw}/conf/`。

### 编码器的位置变过

纯编码器最早在 `common/gwjson`，后来迁到 `hcommon/pkg/pbjson`。看到旧文档或旧提交里的 `gwjson`，对应现在的 `pbjson`；API 名字没变（`Get` / `Put` / `Success` / `Error` / `UnmarshalProto`）。

## 升级流程

1. 在 fork 里升级嵌入的生成器版本，跑它自带的 compat 测试。
2. 用新版本重新生成 fixture，`git diff` 看生成物变化。
3. 跑 `scripts/check-skill.sh`（含 fixture 编译）、`scripts/check-toolchain.sh` 与 `scripts/verify.sh`。
4. 生成物变化可接受 → 更新本表与 [toolchain-and-generation.md](toolchain-and-generation.md)，提交。
5. 变化不可接受 → 回退，记录原因。
