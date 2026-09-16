# Audit 应用规范

适用于 `app/audit/service` 的现有实现。Audit 在 [app 目录说明](../../../app/AGENTS.md) 中标记为“停止更新和做参考，等待后期重构”；本目录只记录后端维护事实，不提供前端规范。

## 开发前检查

- 先确认需求是维护既有 Audit 行为还是重构设计；后者应另有任务与验收。
- Kafka 记录消费、CloudEvent 校验、批处理、提交以及查询维护边界都读取 [后端规范](backend.md)。

## 导航

| 范围 | 规范 | 内容 |
| --- | --- | --- |
| 后端 | [后端规范](backend.md) | 维护状态、Kafka 到 ClickHouse 摄取链与查询边界 |

## 质量检查

- 测试入口与 Kafka、ClickHouse、服务注册及关闭顺序的真实验收边界见 [后端规范](backend.md)。
