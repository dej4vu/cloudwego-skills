# 测试与验证

该不该写测试、写什么、用什么命令验证。

## 验证本 skill

这一节说的是"怎么知道这个 skill 本身是对的"，与下文"目标仓库里怎么验证业务代码"是两回事。

### 三层

| 层 | 回答什么问题 | 怎么做 | 命令 |
|---|---|---|---|
| 结构 | 格式合规、链接不断、没有把过时写法写成目标写法、不含内网信息 | `scripts/check-skill.sh` | 改完任何文件后必跑 |
| 可运行 | fixture 在声明的版本矩阵下能编译、能过测试 | `scripts/verify.sh` + `scripts/check-toolchain.sh` | 改 fixture / 模板 / 版本后必跑 |
| 行为 | agent 读了 skill 之后，是否真的比不读做得更好 | [evals/](../evals/) 的 with / without 对照 | 改规则措辞、加规则、升级 skill 版本后跑 |

前两层只能证明"skill 没坏"，证明不了"skill 有用"。一个语法完全合规、链接全通的 skill 完全可能让 agent 做错事，所以改了规则措辞之后要看第三层。

### 结构校验做了什么

`scripts/check-skill.sh` 与 [Agent Skills 规范](https://agentskills.io/specification) 的 frontmatter 约束一致，可以和官方校验器交叉验证（`pip install skills-ref`，可执行文件名是 `agentskills`，不是包名）。官方校验器有两个使用上的坑：路径要传绝对路径，传 `.` 会因为取不到目录名而误报；校验失败时退出码仍是 0，要看输出里是 `Valid skill` 还是 `Validation failed`：

- frontmatter 只允许 `name` / `description` / `license` / `allowed-tools` / `metadata` / `compatibility`；`name` 小写数字连字符、≤64 字符、与目录同名；`description` ≤1024 字符。版本号放 `metadata.version`，顶层 `version` 会被官方校验器拒绝。
- `SKILL.md` ≤500 行；所有 markdown 相对链接指向存在的文件；`references/` 与 `workflows/` 里没有被遗忘的孤儿文档。
- 过时表述守护：主参考库已经不用的写法（`gormx.Interface`、`validateRequestSession`、`utils.Send*Response`、`c.JSON(consts.StatusOK`、`app/job`）在列出的文档里只能以"遗留 / 已删除 / 对照"的口吻出现。
- `scripts/` 不调用 perl 或 python；文档里没有内网地址、私钥、硬编码口令。

交叉验证的命令（在独立的 venv 里装，本 skill 的脚本不依赖它）：

```bash
agentskills validate "$(pwd)"
```

两者对 frontmatter 的 9 类错误（非法字段、大小写、连字符、目录名不一致、缺失、未闭合、空描述、超长）判定一致。官方校验器不查链接、行数、过时表述、脚本依赖与脱敏，这些是 `check-skill.sh` 多出来的部分。

守护是**启发式**：它按词表判断一行是不是在"肯定"过时写法，换一个近义表达就可能漏过。它只用来拦住高置信度的回退，不能代替行为层。

### 校验器也要被验证

只看"全绿"不能证明校验器有效——它可能只是提前退出了。`scripts/test-check-skill.sh` 在临时副本里逐项注入已知缺陷（非法字段、断链、过时表述、perl 调用、内网地址等 23 种），要求每一种都被报出来，并且未改动的副本必须通过。改了 `check-skill.sh` 之后跑它。

### 行为评测

用例在 [evals/evals.json](../evals/evals.json)，每个用例三部分：`prompt`（真实用户会说的话）、`expected_output`（人读的成功标准）、`assertions`（可核对的断言）。断言里带 `check` 的由 `evals/assertions.sh` 机械判定，其余留给人工。

```bash
# workspace 是 agent 改完后的目标仓库根目录，不是本 skill 目录
evals/assertions.sh <workspace> 1 GetOrderDetail yes   # 网关端点；yes = 该端点需要登录
DAO_TYPE=FileDAO evals/assertions.sh <workspace> 2     # DAO
evals/assertions.sh <workspace> 3                      # Kafka 消费
```

每条断言输出 `PASS` / `FAIL` / `SKIP` 与一行证据，退出码是 `FAIL` 的条数。`SKIP` 表示该工作区里没有对应文件，这条断言无法判定，不计入失败——所以看到一排 `SKIP` 不等于通过。

跑一轮的方法（见 [官方评测指南](https://agentskills.io/skill-creation/evaluating-skills)）：

1. 每个 prompt 跑两次：带 skill 与不带 skill（改进已有 skill 时，基线用上一版快照）。两次用同一个干净工作区、各自独立的会话，避免上一轮的上下文串进来。
2. 对两份产出各跑一次 `assertions.sh`，把结果连同 token 数与耗时记到 skill 目录之外的工作区。
3. 只看**带 skill 过、不带 skill 不过**的断言——那是 skill 真正加分的地方。两边都过的断言说明模型本来就会，可以考虑删掉；两边都不过的，先怀疑断言或用例写错了。
4. 机械断言管不到的（设计取舍、是否过度设计、幂等时机在业务上是否正确）由人看产出。反馈要写成可操作的一句话。
5. 把失败的断言、人的反馈、执行轨迹一起交给模型，让它提出对 `SKILL.md` 的修改；人复核后应用，再跑下一轮。

先从 2–3 个用例开始，跑过一轮、看过真实产出之后再补断言。断言写"handler 的失败分支按来源选入口"这种可观察的行为，不写"输出里出现某句话"这种只匹配措辞的。

### 改完之后

| 改了什么 | 至少要跑 |
|---|---|
| 任何 `.md`、`scripts/` | `scripts/check-skill.sh --structure` |
| `scripts/check-skill.sh` 本身 | `scripts/test-check-skill.sh` |
| fixture、模板、`versions.env` | `scripts/check-skill.sh` + `scripts/check-toolchain.sh` + `scripts/verify.sh` |
| 规则措辞、新增或删除规则 | 以上全部 + 行为评测 |

## 不追求覆盖率 `[主]`

测试按代码类型分级，不为覆盖率写重 mock 测试——那测的是 mock 不是代码。

| 代码类型 | 写测试？ | 验证方式 |
|---|---|---|
| 业务胶水（proto→RPC→DAO 透传、pack 映射） | 否 | `make build` + 运行时验证 |
| 跨 RPC 链路、重 mock 场景 | 否 | 集成环境验证 |
| 纯逻辑（算法、评分、过滤、状态机、option 构造） | **是** | TDD |
| 生成的 `_test.go` 桩 | 保留 | 可不填充 |

## 三类标签 `[主]`

动手前先给函数打标签：

| 标签 | 适用 | 验证 |
|---|---|---|
| `[logic]` | 零 mock 能测的纯逻辑，有真实分支/边界/计算 | TDD，`make test` |
| `[glue]` | 透传、映射、跨 RPC、重 mock | `make build` |
| `[mixed]` | 既有纯逻辑又有 IO | 把纯逻辑抽成独立函数标 `[logic]`，编排按 `[glue]` |

拿不准默认 `[glue]`。"纯逻辑"不等于"必测"——trivial 的转发或映射按 `[glue]` 跳过。

## 必须先写测试的 `[主]`

评分与排序、推荐算法、风控规则计算这类**算法正确性**代码必须先写测试：手动很难覆盖边界，而零 mock 的测试成本极低。

## 测试 outbox 与幂等 `[主]`

outbox worker 和支付幂等是少数值得用 fake 测试的编排逻辑——用窄接口注入 fake store / fake publisher，不 mock 整个 DAO：

```go
type fakePublisher struct{ published []string }
func (p *fakePublisher) PublishPayload(_ context.Context, _ int64, payload string) error {
    p.published = append(p.published, payload)
    return nil
}

func TestOutboxPublishesOnce(t *testing.T) {
    store := newFakeStore().withDue("evt-1")
    pub := &fakePublisher{}
    w := &outboxWorker{store: store, publisher: pub}
    w.processDue(context.Background())
    if len(pub.published) != 1 {
        t.Fatalf("want 1 published, got %d", len(pub.published))
    }
}
```

## 测试 SSE 桥接

流式桥接是少数值得用 fake 测的网关逻辑：把 `service.Run` 依赖的下游调用抽成一个可替换的函数变量，返回按脚本吐事件的假 stream，就能在不启动 RPC 的情况下验证事件顺序、终止语义和 `partIndex=0` 是否保留。

`hertz-contrib/sse` 走 chunked writer，依赖真实网络连接，`ut.PerformRequest` 的 recorder 下会空指针。所以桥接测试必须起真实监听、走一次完整 HTTP 往返；整个包共用一个网关实例（`TestMain`），否则优雅关闭要为每个用例等 5 秒。

参考 [fixtures/stream-sse/app/apigw/gateway_test.go](../fixtures/stream-sse/app/apigw/gateway_test.go)，至少覆盖：

| 用例 | 验证 |
|---|---|
| 正常流 | 响应头 `text/event-stream`，事件顺序，`seq` 从 1 连续递增，`delta` 里 `partIndex=0` 存在 |
| 下游发 `error` 事件 | 客户端收到 `error` 事件而不是断流 |
| `done` / `error` 之后还有事件 | 不再转发 |
| 流中途 `Recv` 报错 | 网关补一个 `error` 事件 |
| `Recv` 报的是 Kitex 业务错误 | 沿用下游的错误码与消息 |
| 建流失败 | 发生在 SSE 开始之前，返回普通 JSON 信封 |

## 验证命令 `[主]`

Makefile 存在时**禁止**裸 `go build ./...` / `go test ./...`——Makefile 统一处理多 module 和生成依赖。

| 用途 | 命令 |
|---|---|
| 编译 | `make build svc={service}` |
| 测试 | `make test svc={service}` |
| 全量 tidy | `make tidy` |
| 格式化 | `gofmt -w .` + `goimports -w .` |
| 静态检查 | `go vet ./...` |

## 每个逻辑单元的收尾 `[主]`

1. `make build` 通过。
2. 有 `[logic]` 的跑 `make test`。
3. 改了生成流程的，重新生成后 `git diff` 干净。
4. 每完成一个逻辑单元提交一次，提交信息说清"做了什么"。
- `SKILL.md` ≤500 行；所有 markdown 相对链接指向存在的文件；`references/` 与 `workflows/` 里没有被遗忘的孤儿文档。
