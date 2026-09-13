# CAP：PoW challenge 与一次性验证

本主题对应 `security/cap` 的 CAP 人机验证能力，不是通用权限 capability 模型。它不拥有登录、账号、RBAC、OpenFGA 或业务风控策略。

## 完整流程

1. `New(config, redisClient)` 使用生成配置和共享 Redis；配置在依赖之前。签名密钥至少 16 字节；challenge 参数与 TTL 有边界检查。
2. `CreateChallenge` 生成带 c/s/d、nonce、毫秒 exp/iat 和可选 scope 的 HS256 签名 challenge。此步骤不把 challenge 本体保存到 Redis。
3. `RedeemChallengeWithScope` 验证签名、格式、过期、服务端 scope 与解题结果。scope 来自服务端策略，不能把客户端声称的 scope 当作期望值。
4. Redis Lua 原子写入已使用 nonce 标记与验证 token，防止并发重复兑换；存储 token key 使用 secret 的 SHA-256 摘要。
5. `ValidateToken` 通过 GETDEL 一次性消费；需要用途隔离时使用 `ValidateTokenWithScope`，仅 scope 匹配才删除。

默认 key 前缀为 `cap:v2:`，challenge TTL 为 10 分钟、token TTL 为 20 分钟。修改前缀、签名密钥、协议参数或 TTL 须评估旧 challenge/token 和前端 widget 兼容性，不能在业务层复制 Redis key 拼装规则。

## HTTP 与协议边界

嵌入路由为 `POST /cap/challenge` 和 `POST /cap/redeem`；前者返回 `challenge: {c,s,d}`、签名 `token` 和毫秒 `expires`，后者接收 `token`、整数数组 `solutions`。解码拒绝未知字段、重复字段等歧义 JSON；失败正文不暴露 Redis 或签名内部错误。业务认证白名单使用导出的 operation 常量，不重新拼旧 `/v1/cap` 路径。

当前协议 v2 不读取旧有状态实现的 challenge/token 记录。基础 SHA-256 wire 与 capjs-core 0.1.x 对齐，已有 widget 0.1.57 的本地 HTTP 互操作测试；不据此宣称支持 RSW、format-2、Cap Standalone 或 siteverify。多实例共享同一密钥与 Redis 时，一次性兑换／消费约束应跨实例成立；本地测试不等同于真实浏览器和部署环境验收。

## 失败与检查

错误解题不应提前消耗 challenge；并发兑换最多发放一枚 token；Redis 错误返回失败而非降级放行；错误 scope 不消耗可供正确 scope 使用的 token。公开兑换响应区分 `Success=false` 与内部依赖错误。

依据：[cap.go](../../../../security/cap/cap.go)、[协议向量测试](../../../../security/cap/cap_test.go)、[跨实例与一次性消费测试](../../../../security/cap/cap_integration_test.go)。入口：`go test ./security/cap`。

历史 `security/cap/AGENTS.md` 对“challenge 与 token 都基于 Redis 存储”的概括不准确；以本主题和当前实现的签名 challenge／一次性状态边界为准。
