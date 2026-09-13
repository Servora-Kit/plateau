# biz 层

业务文件按确认顺序书写：import → 必要常量/变量 → 一组或多组 `XxxRepo` 接口 → `XxxUsecase` → `NewXxxUsecase` → Usecase 方法。无常量或变量时不创建占位。Example 的 [user.go](../../../../app/example/service/internal/biz/user.go) 同时展示领域错误、`UserScope`、`UserRepo` 和 `UserUsecase`；IAM 的 [user.go](../../../../app/iam/service/internal/biz/user.go) 是相同顺序的现行例子。

Repo 接口仅表达业务需要，data 用私有实现满足它。Usecase 负责领域校验、身份/租户作用域、并发和生命周期分支、业务错误翻译。例如 Example 的 `UpdateUser` 处理 allow-missing、etag 和 `ErrMutationMiss`，再调用 Repo。不要 import `internal/data`、Ent 或 SQL；不要让 data 猜测授权、tenant 或幂等语义。

`biz.go` 只放 ProviderSet；业务方法不塞进其中。密码等输入专有敏感字段须在 biz 完成哈希并从资源中清除，见 [CreateUser](../../../../app/example/service/internal/biz/user.go)。
