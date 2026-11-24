# Audit 服务规范索引

适用于 `app/audit/service`。消费、ClickHouse `ch` 结构体持久化、游标防篡改与内部服务鉴权契约见 [后端规范](backend.md)；共通四层组织遵循 [共享微服务规范](../service/backend/index.md)。

## 开发前检查

- 改动表列或 DDL 前，同步核对 `internal/data/schema/event.go` 中 `EventRow` 的 `ch:"..."` 标签与 `CREATE TABLE` 全部列顺序/类型完全一致。
- 改动过滤条件前，同步核对 `internal/biz/page_token.go` 的 `queryFingerprint` 与 `internal/data/event.go` 的 `EventRepo.List` 参数绑定。
- 改动鉴权配置前，核对 `manifests/openfga/audit.fga` 与 `configs/{local,docker}/jwt_authn.yaml`。

## 质量门禁

- `GOWORK=off go test -race -count=1 ./app/audit/service/...`
- `just openfga-model-validate && just openfga-model-test`
