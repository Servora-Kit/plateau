# 共享规则规划

适用于 optionmerge、ruleplan 和插件测试设施。职责是可复用的声明读取、合并与输出规划；AuthN/AuthZ 特定错误、字段约束和 Go 输出语句留在各命令。

## 合并和克隆

[optionmerge.Merge](../../../../internal/codegen/optionmerge/optionmerge.go) 约定 mode 为字段 1：方法规则 mode 非零时整体替换 service default；方法不存在或 mode 为零时继承有效 default；两侧都没有非零 mode 时无规则。结果深克隆，不做字段逐个叠加，也不保留被替换 default 的目标。

## Build 顺序

[ruleplan.Build](../../../../internal/codegen/ruleplan/ruleplan.go)：
1. 遍历请求内文件，索引 service 与 method 显式声明并执行 ValidateDeclared。
2. 只对 file.Generate 的文件形成输出；按生成目录分组并拒绝同一目录内冲突的 Go import path/package name。
3. 合并后调用 AcceptMerged 做领域验收；按完整 `/ServiceFullName/Method` 去重。
4. 排序目录和 operation 后返回 Group。没有有效规则的目录不产生空输出文件。

不能只校验最终留下的声明，也不能依赖 map 遍历顺序产生代码。扩展共享规划应保持错误定位中的文件、service、method/operation 信息。

[plugintest](../../../../internal/codegen/plugintest/plugintest.go) 为命令测试提供生成结果编译与行为验证。入口为两个 cmd 插件测试；重构规划同时运行两组，避免只满足一个注解家族。规则运行时聚合与生成合并是不同阶段，见 [AuthN](../security/authN.md)、[AuthZ](../security/authZ.md)。
