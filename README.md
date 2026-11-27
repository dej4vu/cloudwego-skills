# cloudwego-skills

面向 AI 编程 agent 的 [Agent Skill](https://agentskills.io)，规范 [CloudWeGo](https://github.com/cloudwego) **Hertz（HTTP 网关）**与 **Kitex（RPC 服务）**的业务开发：哪些代码是生成物、业务逻辑写在哪、IDL 怎么改、改完怎么验证。

适用于使用 cwgo / hz / kitex 代码生成、Protobuf IDL、`rpc_gen/` / `hertz_gen/` 生成目录、网关到 RPC 调用链的 Go 仓库。规则基线提炼自两个生产代码库的实践：标注 `[主]` 的条目来自较新的一套（主参考库），另一套演进前的形态保留在 [case-studies/legacy.md](case-studies/legacy.md) 作对照。已按公开要求脱敏，不含内部项目信息。

## 它解决什么问题

让 agent 在这类仓库里少犯典型错误：

- **IDL 先行**：先改 `.proto`、重新生成，最后写业务；不先写 Go 再补 IDL。
- **生成物不手改**：业务逻辑只写在 `biz/service/{method}.go`，`kitex_gen/` / `hertz_gen/` / 生成的 router 只能通过重新生成变更（流式 handler 桥接是唯一登记过的例外）。
- **网关不碰数据**：网关只做接入、认证、路由、pack；每张表只有一个 owner service。
- **响应统一信封**：不用 `c.JSON` + `encoding/json` 输出 proto 响应（`int64` 丢精度、零值被 `omitempty` 吃掉）。
- **错误带稳定码**：不靠 `strings.Contains(err.Error(), ...)` 匹配错误文案。
- **写路径一致性**：业务事件不在事务外同步发 Kafka，走 outbox + 幂等消费。

完整规则见 [SKILL.md](SKILL.md) 的「每次都要守的规则」与「不要做的事」。

## 安装

把本仓库放到你的 agent 的 skills 目录即可，例如：

```bash
# Claude Code（个人级）
git clone https://github.com/dej4vu/cloudwego-skills.git ~/.claude/skills/cloudwego-skills

# Codex（个人级）
git clone https://github.com/dej4vu/cloudwego-skills.git ~/.codex/skills/cloudwego-skills
```

也可以放进项目级 skills 目录，或把 [SKILL.md](SKILL.md) 的内容并入项目的 `AGENTS.md` / `CLAUDE.md`。规范优先级：**目标仓库自身的约定 > 本 skill**；冲突时以仓库为准。

## 仓库结构

| 目录 | 内容 |
|---|---|
| [SKILL.md](SKILL.md) | 入口：任务分类路由、全局规则、禁止事项 |
| [references/](references/) | 13 篇参考文档：架构、工具链与生成、生成物边界、IDL 契约、Kitex 服务、Hertz 网关、数据与缓存、异步与幂等、SSE 流式、错误/认证/可观测、测试与验证、排障、版本兼容 |
| [workflows/](workflows/) | 6 个端到端操作流程：新服务、新 RPC 方法、新 HTTP 端点、流式端点、outbox 事件、支付回调 |
| [templates/](templates/) | 可直接复用的代码片段与修好的 cwgo client yaml 模板 |
| [fixtures/](fixtures/) | 4 个可运行的最小工程：`kitex-greet`、`hertz-greet`、`stream-sse`、`outbox-idempotency` |
| [case-studies/](case-studies/) | 主参考库与对照库的实践对照（已脱敏） |
| [evals/](evals/) | 4 个行为评测用例、25 条断言，其中 18 条可由 [evals/assertions.sh](evals/assertions.sh) 机械判定 |
| [scripts/](scripts/) | 自检、工具链核对与 fixture 验证脚本 |

## 验证

脚本只依赖 **bash + Go 工具链**与标准 unix 工具（grep / sed / awk / find），不依赖 perl、python、jq：

```bash
scripts/check-skill.sh              # 结构校验 + 过时表述守护 + fixture 编译
scripts/check-skill.sh --structure  # 只做结构与守护，不编译
scripts/test-check-skill.sh         # 校验器的故障注入自测（25 个用例）
scripts/check-toolchain.sh          # 本机 cwgo / hz / kitex 与验证矩阵对齐
scripts/verify.sh                   # 全部 fixture：tidy / build / vet / test / gofmt
scripts/verify.sh --build-only      # 只编译，不改任何文件
```

fixture 需要 Go 1.25+。可用官方校验器交叉验证：`agentskills validate <本仓库绝对路径>`（pip 包 `skills-ref`）。

## 版本基线

规则与 fixture 在以下版本下验证（完整矩阵与升级注意事项见 [references/compatibility.md](references/compatibility.md)）：

- cwgo `v0.1.3`（[维护中的 fork](https://github.com/dej4vu/cwgo)，上游 `cloudwego/cwgo` 已归档；内嵌 Kitex 生成器 `v0.16.3`、hz `v0.9.7`）
- Kitex 运行时 `v0.16.4`、Hertz 运行时 `v0.10.6`

cwgo 仓库一律用 cwgo 生成，避免 PATH 里的原生 `kitex` / `hz` 版本不一致造成大量无关 diff；网关侧 fork 与原生 hz 两条路线不要在一个仓库里混用。

## 已知边界

- **SSE 流式**协议仍在调整：[references/streaming-sse.md](references/streaming-sse.md)、[workflows/add-streaming-endpoint.md](workflows/add-streaming-endpoint.md) 与 `stream-sse` fixture 暂不作为规范。
- `fixtures/hertz-greet` 与 [templates/hertz/resp.go.tmpl](templates/hertz/resp.go.tmpl) 仍是旧的三层匹配响应实现，与 [references/hertz-gateway.md](references/hertz-gateway.md) 的 `responsex` 两入口形态待同步。
- [evals/](evals/) 的断言判定逻辑已验证；with/without skill 的完整行为对照评测尚未跑完。

## 许可证

[Apache-2.0](LICENSE)
