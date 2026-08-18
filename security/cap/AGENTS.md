# AGENTS.md - pkg/cap/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-22 | Updated: 2026-03-22 -->

## 模块目的

提供基于 Redis 的内嵌式 Cap PoW 人机验证服务端，实现 challenge 生成、兑换与验证 token 发放的完整链路。

## 当前文件

- `cap.go`：Cap 服务主实现与配置
- `cap_test.go`：挑战生成与校验测试

## 当前实现事实

- challenge 与 token 都基于 Redis 存储，并使用固定 key 前缀管理
- 配置项覆盖 challenge 数量、字符集/长度、难度、过期时间等
- redeem 成功后会发放验证 token，供后续链路消费
- 该包表达的是“挑战生命周期服务”，不是通用认证框架

## 边界约束

- 不在本包中承载账号登录、JWT、RBAC 或 OpenFGA 授权逻辑
- 不把 Redis 操作细节扩散到业务层；业务层应通过 Cap 服务接口使用能力
- 不把 challenge/token key 规则复制到其他模块里手写维护

## 常见反模式

- 把 Cap 当成通用登录风控容器，持续往里塞业务规则
- 绕过服务接口直接读写 Redis key
- 生成 challenge 与兑换 token 使用不一致的 TTL / key 前缀约定

## 维护提示

- 若调整 challenge 或 token key 前缀，需同步评估历史数据兼容性
- 若修改难度与过期策略，需确认前端交互和服务端验证窗口仍匹配
