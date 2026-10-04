# 领域层

文件顺序：import → 必要常量/变量 → Repo 接口 → Usecase → 构造函数 → Usecase 方法；无常量或变量时不创建占位。

Repo port 仅表达领域需要，由 data 私有实现满足。Usecase 负责领域校验、身份与租户作用域、并发控制、生命周期分支和错误翻译；不 import data、Ent 或 SQL，不让 data 猜测授权或幂等语义。

`biz.go` 只放 ProviderSet。输入专用的敏感数据在 biz 处理后从资源中清除，密码哈希不下放到 data。
