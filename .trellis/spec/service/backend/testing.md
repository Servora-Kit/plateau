# 测试与质量检查

测试贴近规则所属层：biz 用 fake Repo 覆盖生命周期和错误分支；data 验证真实存储语义；service 验证请求规范化与接口契约；server 集成测试验证 transport 注册。

列表、分页、过滤或 CRUD 映射变化须覆盖应用消费及相应框架合同；框架合同测试不能替代应用端到端验证，合同归属见 [CRUD](crud.md)。

代码改动至少运行受影响 Go package 的测试和项目要求的 lint；涉及数据库、消息系统、授权或 HTTP/gRPC 边界时补充相应集成检查。命令见 [索引](index.md)。
