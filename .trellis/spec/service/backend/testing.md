# 测试与质量检查

优先让测试贴近拥有规则的层：biz 用 fake Repo 断言生命周期和错误分支，data 测试真实 Ent/存储语义，service 测试请求规范化和接口契约，server 集成测试验证已注册 transport。Example 的 [biz 单元测试](../../../../app/example/service/internal/biz/user_test.go)、[service 集成测试](../../../../app/example/service/internal/service/user_integration_test.go) 是可运行入口；IAM 还含 [data session 集成测试](../../../../app/iam/service/internal/data/session_integration_test.go)。

修改列表、分页、过滤或 CRUD 映射时，除应用测试外检查 Servora 对应合同测试；当前框架存在 [Ent CRUD 集成合同测试](../../../../../servora/contrib/db/entgo/crud/live_contract_integration_test.go)。该路径是独立仓库证据，不能据此宣称 Plateau 已做端到端验收。

服务质量命令由服务 `justfile` 和根 [app 命令](../../../../app/AGENTS.md) 决定。仅文档任务不运行全套构建；改动代码后至少运行受影响 Go package 的测试和项目要求的 lint，再按是否涉及数据库、Kafka、OpenFGA 或 HTTP/gRPC 决定集成检查。
