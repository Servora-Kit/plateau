<p align="center">
  <img src="./docs/assets/logo.png" alt="Plateau" width="350"/>
</p>


<h1 align="center">Plateau</h1>

<p align="center">
  构造像高原一样稳固强大的微服务
</p>


简体中文

> 本项目是 [Servora](https://github.com/Servora-Kit/servora) 框架的最佳实践

`plateau` 当前包含具体 JWT AuthN、OpenFGA AuthZ、JWT/OpenFGA 基础能力、安全 Proto/codegen、IAM 微服务、Audit 微服务、Example CRUD 服务及其 Web 入口。


## 微服务

- **IAM**（`app/iam/service/`）：IAM 微服务
  - 为 Plateau 整个平台所有微服务提供中性化认证授权服务

- **Example**（`app/example/service/`）：Servora CRUD 生态示例服务
  - 是 servora 的经典用法，也是官方推荐的代码布局

- **Audit**（`app/audit/service/`）：全链路审计日志服务
  - 基于 Kafka 消费审计事件
  - ClickHouse 持久化存储
  - 审计日志查询 API


## 基础设施 provider 接线

Servora/Plateau 的基础 client provider 使用 Proto 配置作为连接参数来源，不把业务 logger 混入位置参数：

- Redis：`redis.New(cfg)` 返回官方 Redis client 和 cleanup；业务日志由 IAM data/bootstrap 边界决定。
- Kafka：`kafka.NewClientOptional(ctx, cfg, kafka.WithSlogLogger(log.With("scope", "audit/kafka")), kgo.ConsumerGroup(group), kgo.ConsumeTopics(topic))`；未传日志 Option 时不绑定全局业务 logger。
- ClickHouse：`clickhouse.NewConnOptional(ctx, cfg)` 保持三态返回；compression 支持空值/`none`、`lz4`、`zstd`，未知值直接返回配置错误。

从旧签名迁移时，删除 Redis/Kafka/ClickHouse 构造函数中的 logger 位置参数；Kafka 原生日志改为显式 `WithSlogLogger` Option，业务 logger scope 在调用方建立。

## CAP 人机验证

`security/cap` 是可由 IAM 或其他微服务直接挂载的 Go 模块，公开 `/cap/challenge` 与 `/cap/redeem`，不依赖外部 Cap Standalone 服务。使用方在组合根把生成的 `*capv1.CAP` 配置和共享 Redis client 传入 `cap.New(capConfig, redisClient)`。

CAP 配置的 `signing_secret` 必须由部署环境提供，至少 16 字节，并在所有实例间保持一致；其余 PoW、TTL 和 Redis namespace 使用生成配置中的默认值。当前实现跟进 `capjs-core` 的基础 HS256 challenge JWT、SHA-256 PoW、可选 scope 与 Redis replay nonce 语义，不实现 instrumentation、headless 检测、RSW 或 format-2。

从旧状态型实现切换时不读取旧 `cap:challenge:*` 和 `cap:token:*` 数据；切换前签发的 challenge token 与一次性 verification token 将失效。后端与固定版本 widget 应协调发布，客户端收到失败结果后重新获取 challenge。
