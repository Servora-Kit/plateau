# 可选 Provider 与 Capability Adapter

OpenFGA、ClickHouse、Kafka、Redis、通用缓存以及 Ent/GORM 数据库与 CRUD 适配均归 [Plateau infra](../../plateau/infra/index.md)，不在微内核恢复重型存储与 ORM 依赖。

当前 `../servora/contrib/` 仅保留微内核自身依赖的 `contrib/k8s/`（供 `core/registry/kubernetes.go` 使用）。若评估新增微内核级可选扩展，须保持零 ORM/数据库驱动耦合，并通过 `go test ./contrib/...` 验证。
