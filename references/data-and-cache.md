# 数据访问与缓存

DAO 怎么组织、什么时候走缓存、哪些操作允许退回原生 GORM。

## 一个 DAO 一个文件 `[主]`

`biz/dao/{entity}_dao.go`，不把多个 DAO 塞进同一文件。没有独立的 `repo` 目录，也没有独立的 Repo 结构体——数据访问统一在 DAO 层，缓存能力以协作者的形式内嵌在 DAO 里。

## DAO 是"执行器 + 缓存协作者"的组合根 `[主]`

```go
type FileDAO struct {
    g          gorm.Interface[*model.File]                     // GORM 官方泛型执行器
    cacheRepo  *cachex.BaseCachedRepository[model.File, int64] // 可选：热点实体缓存
    countCache *cachex.CountCache                             // 可选：列表总数缓存
}

func NewFileDAO() *FileDAO {
    return &FileDAO{
        g: gorm.G[*model.File](mysql.DB),
        cacheRepo: cachex.NewBaseCachedRepository(
            mysql.DB,
            rdal.RedisClient,
            func(id int64) string { return fmt.Sprintf("file:file:v1:id:%d", id) },
            func(item *model.File) int64 { return item.ID },
            cachex.CacheConfig{
                Namespace:  "file:file:v1",
                DefaultTTL: 10 * time.Minute,
                OrderField: "id",
            },
        ),
    }
}
```

| 字段 | 必选 | 说明 |
|---|---|---|
| `g gorm.Interface[*model.X]` | 有单表读写时必选 | 由 `gorm.G[*model.X](mysql.DB)` 构造 |
| `cacheRepo *cachex.BaseCachedRepository[T, ID]` | 可选 | 按主键的 cache-aside 读与失效 |
| `countCache *cachex.CountCache` | 可选 | 列表总数缓存 |
| `redis *redis.Client` | 可选 | 仅当需要 cachex 表达不了的操作（锁、计数器、版本 token）；`cacheRepo` 自带 Redis 客户端，不要重复持有 |

**禁止**在 DAO 里持有 `db *gorm.DB` 字段。自研查询器 `gormx.Interface` / `gormx.G` 已删除，仓库用 `make check-dao` 守护这两条。`gormx` 现在只剩 not-found 辅助函数和连接池配置。

缓存按实体声明：只有满足"读多写少、允许短时间陈旧、能明确列出失效 key"的实体才接 `cacheRepo`，不要为了结构统一给每个 DAO 都塞一个空缓存字段。

## 方法命名即缓存语义 `[主]`

| 前缀 | 缓存 | 实现 |
|---|---|---|
| `FindBy...` | 无，直查 DB | `d.g.Where(...).First(ctx)` / `Find(ctx)` |
| `GetBy...` | cache-aside，允许陈旧 | `d.cacheRepo.FindByID` / `FindByCondition` |
| `ListBy...` / `List...` | 直查 DB；总数可接 `countCache` | `d.g.Where(...)`；总数走 `CountCache` |
| `Update...` / `Delete...` | DB 写，成功后失效缓存 | `d.g` 写 + 显式 `InvalidateCache` |

新增缓存方法必须用 `GetBy...` 并在注释里写 `// cached`；不要让两个名字相近的方法一个带缓存、一个不带却看不出区别。

> 存量代码里有 2 处命名与缓存语义不一致，都是未按规范命名的遗留，新代码不要照搬：`FileDAO.FindByID` 走的是缓存却叫 `FindBy`；`SmsCodeDAO.GetByRequestID` 直查 `d.g`（该 DAO 没有 `cacheRepo`）却叫 `GetBy`。

## 统一从 `d.g` 起链 `[主]`

```go
// 列表
items, err := d.g.Where("status = ?", status).Order("id DESC").Find(ctx)

// 总数
total, err := d.g.Where("status = ?", status).Count(ctx, "id")

// 创建：T 已经是指针，所以传 **model.X
item := &model.User{Name: name}
if err := d.g.Create(ctx, &item); err != nil { /* ... */ }

// 条件更新：官方 Update 只接受单列，多列用 clause.Assignments + Set；返回值是影响行数
rows, err := d.g.Where("id = ? AND status = ?", id, StatusPending).
    Set(clause.Assignments(map[string]any{
        "status":     StatusPaid,
        "updated_at": time.Now(),
    })).
    Update(ctx)
if err == nil && rows > 0 {
    // 推进成功（CAS 语义）
}
```

### 复用过滤条件

helper 返回 `gorm.ChainInterface[*model.X]`。`gorm.Interface` 不能直接赋给 `gorm.ChainInterface`，所以用中性条件起步：

```go
func (d *XDAO) adminQuery(filter Filter) gorm.ChainInterface[*model.X] {
    q := d.g.Where("1 = 1")
    if filter.Status != "" {
        q = q.Where("status = ?", filter.Status)
    }
    return q
}
```

`Count` 与 `Find` 各调用一次 helper，禁止复用同一个已链式化的对象做多次查询。

### 事务、行锁与复杂 SQL 是明确例外

GORM 官方泛型没有 `Transaction` 入口。事务、行锁、`OnConflict`、原生 SQL、聚合查询允许在**方法内**使用 `mysql.DB` / 事务回调参数 `tx`：

```go
err := mysql.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    var item model.User
    if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
        Where("id = ?", id).First(&item).Error; err != nil {
        return err
    }
    return tx.Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
})
```

事务内需要泛型 API 时显式构造 `g := gorm.G[*model.X](tx)`，但不要把 `tx` 存进 DAO 字段。以事务和多表写入为主的聚合型 Store（`UserStore`、`PaymentStore`）可以不声明 `g`，同样不得持有 `db *gorm.DB` 字段。

## not-found 必须显式表达 `[主]`

DAO 不能把"记录不存在"一律吞成 `nil, nil`，那会让"查不到"和"本来就可以没有"共用同一个返回值：

| 场景 | 写法 | 返回 |
|---|---|---|
| 调用方要求记录存在 | `gormx.WrapNotFound(item, err)` | `errno.Err_RecordNotFound` |
| 不存在是正常业务分支（幂等查询、可选关联） | `gormx.IgnoreNotFound(item, err)` | `nil, nil` |
| 事务内"不存在才插入" | 保留 `gormx.IsNotFound(err)` 做控制流 | 原始 err |

```go
// 必存在查询
item, err := d.g.Where("id = ?", id).First(ctx)
if err != nil && !gormx.IsNotFound(err) {
    return nil, fmt.Errorf("failed to find user: %w", err)
}
return gormx.WrapNotFound(item, err)
```

Service 层要把 `errno.Err_RecordNotFound` 映射成领域错误（如 `Err_ProductUnavailable`），不能当成 `Err_ServiceErr`。

## 缓存协作 `[主]`

### 读

- `cacheRepo.FindByID` 用于按主键的 cache-aside 读；`FindByCondition` 只用于能稳定序列化的简单条件。
- 写后读自己的写、事务内读、强一致读走 `d.g`，不走缓存。

### 写

写操作统一走 `d.g`，**成功后**再失效缓存：

```go
if _, err := d.g.Where("id = ?", id).
    Set(clause.Assignments(map[string]any{"status": status})).
    Update(ctx); err != nil {
    return fmt.Errorf("failed to update status: %w", err)
}
_ = d.cacheRepo.InvalidateCache(ctx, id)
```

不要用 `cacheRepo.Create/Update/Delete` 写库：它们内部持有自己的 `*gorm.DB`，且 `Updates(entity)` 会跳过零值字段。`cacheRepo` 只当读缓存和失效器。不在事务内删缓存，也不在 DB 写失败时删缓存。

### 计数缓存

`countCache` 只用于"总数短时间陈旧可接受"的后台列表（如登录审计）。Key 必须带实体命名空间，TTL 1~3 分钟：

```go
cc := cachex.NewCountCache(rdal.RedisClient, time.Minute)
cc.SetKeyPrefix("user:login_log:v1:count")
```

过滤条件组合无法在写入时全部失效，所以余额、验证码、频控、权限判断等强一致或安全敏感的计数不要缓存。

### 规范领先于代码的地方

`CacheConfig.Namespace` 规范要求必填且格式为 `<service>:<entity>:<schema-version>`，但 `cachex` 代码并不校验，新接入缓存时自己守住。现状只有 `file` 接了 `cacheRepo`、`user` 的登录审计列表接了 `countCache`，其余 DAO 保持 DB-only。

## 缓存什么、不缓存什么 `[主]`

| 场景 | 建议 |
|---|---|
| 热点只读实体（商品、文件元数据、配置） | `cacheRepo`，TTL 3~10 分钟 |
| 后台列表总数 | `countCache`，TTL 1~3 分钟 |
| 同一请求内重复查询 | 请求内本地缓存或请求上下文传递，不重复查 |
| 余额、验证码、频控、鉴权 | 不套通用实体缓存 |
| 删除后必须立即不可见的数据 | 删除同步失效，先排除后硬删 |
| 强一致读 | 不缓存 |

## 模型 `[主]`

- 定义放 `biz/model/{entity}.go`。
- **不在代码里定义索引**。索引由 migration 或 DBA 管理，GORM model 只描述列。
- 跨服务的表只能在 owner service 的 model 里定义；别的服务通过 RPC 读。

## 表归属 `[主]`

每张表只有一个 owner service。整个项目每个环境共用一个数据库，这**不**意味着可以跨服务读写彼此的表。需要别人的数据就发 RPC，哪怕表就在同一个库里。

## 与对照库的差异

两个仓库的 DAO 执行器相同，都是 GORM 官方泛型 `gorm.Interface[*model.X]`，都禁止 `biz/repo/`。差别在约定是否成文：

| | 对照库 | 主参考库 |
|---|---|---|
| 方法命名 | `Find*` 和 `Get*` 都可能走缓存，看名字分不出来 | `GetBy*` = 走缓存并标 `// cached`，`FindBy*` = 直查 |
| not-found | 有 `WrapNotFound` 但没有统一要求 | 三种场景各有指定写法 |
| 多列更新 | 未使用 `Set(clause.Assignments(...))` | 统一写法 |
| 守护 | 无 | `make check-dao` |
| 缓存接入面 | 13 个 DAO 接了 `cachex` | 2 个（`file`、`user` 登录审计） |

新代码按主参考库的约定写，尤其是命名：对照库里 `GetById` 走的是缓存这件事只能读实现才知道。
