# Go 版本与语法调研

## 官方版本事实

- 官方下载 JSON 返回 `version: go1.27.3`、`stable: true`，含 darwin/arm64 与 Linux 构建：[Go downloads](https://go.dev/dl/?mode=json)。
- 本机 `GOTOOLCHAIN=local go version` 实测 `go version go1.27.3 darwin/arm64`。版本可得性已直接确认，无需安装或修改全局工具配置。
- Go 1.27 的语言改动包括泛型方法、结构体字面量初始化提升字段、更多上下文的函数类型推导：[Go 1.27 release notes](https://go.dev/doc/go1.27)。两仓原本已要求 1.27.1，因此升级到 1.27.3 本身不解锁新的语言版本；优化也可以采用此前已发布、仓库尚未完全使用的简洁写法。
- `new(expr)` 与 `errors.AsType[E error]` 来自 Go 1.26：[Go 1.26 release notes](https://go.dev/doc/go1.26)、[Go 1.26 API manifest](https://github.com/golang/go/blob/go1.27.3/api/go1.26.txt)、[errors.AsType](https://pkg.go.dev/errors#AsType)。Context7 已以 `/golang/go/go1.27.3` 查询并返回 API 证据。

## 计划采纳

### A. `new(expr)` 替代仅为取址而存在的变量或私有测试 helper

- `app/example/service/internal/biz/user.go:249-258`：`hashTemporaryPassword` 的成功分支从 `value := string(hash); return &value, nil` 简化为 `return new(string(hash)), nil`。nil、bcrypt 错误处理不变，字符串转换次数与指针独立性保持。
- `app/iam/service/internal/biz/account_test.go:298`：移除 `stringPtr`，所有真实调用改为 `new(...)`。
- `app/example/service/internal/service/user_integration_test.go:418`：移除 `stringPointer`，所有真实调用改为 `new(...)`。
- `../servora/core/crud/filter_internal_test.go:282-283`：移除 `stringPointer`、`int32Pointer`，更新所有调用。原 `int32Pointer(0)` 必须写成 `new(int32(0))`，保持 `*int32`，不能写成默认推导为 `*int` 的 `new(0)`。
- 两仓已有新写法先例：`app/iam/service/internal/biz/authentication_test.go:48`、`../servora/cmd/protoc-gen-typescript-http/internal/plugin/generate.go:58-59`。
- 实施前使用 LSP references 查全 helper 调用；图影响分析后再删除，删除后不保留别名。

### B. 单个 `errors.As` 判断改为 `errors.AsType`

- `app/example/service/internal/biz/user.go:279-282`：`var apiError *kerrors.Error; if errors.As(err, &apiError)` 改为 `if _, ok := errors.AsType[*kerrors.Error](err); ok`，继续返回原始 `err`，不返回解包后的错误。
- `AsType` 遍历包装/多错误树、支持自定义 `As(any) bool`；与这个只读取匹配布尔值的用法一致。官方接口说明由本机 `go doc errors.AsType` 复核。
- 不重写 `security/authz/openfga/middleware.go:134-166` 或 `app/iam/service/internal/oidc/op_auth_storage.go:280-285` 的多项 `||` 判断，避免引入额外声明或改变短路顺序。

### C. 重复前缀处理改为 `strings.CutPrefix`

- `app/audit/service/internal/data/batch_writer.go:259-270`：`serviceFromSource` 中 `HasPrefix(source, "//")` 后再 `TrimPrefix` 的单个分支改为 `if service, ok := strings.CutPrefix(source, "//"); ok { return service }`。
- 其他 legacy path 解析与 Audit 行为不变。仅做这一机械等价修改，不恢复或扩展已停止维护的 Audit 功能（`README.md:29`）。
- 现有行为测试：`app/audit/service/internal/data/consumer_test.go:58-72` 的 `TestServiceFromSource`。
- 官方 API：[strings.CutPrefix](https://pkg.go.dev/strings#CutPrefix)。

## 不采纳的扩展

- `internal/codegen/plugintest/plugintest.go:135-142`、`../servora/cmd/internal/plugintest/plugintest.go:113-121` 等预分配 map key 收集不改成 `slices.Sorted(maps.Keys(...))`：现有代码准确预分配且空输入返回非 nil 空切片，较短写法没有足够收益证明值得改变这些性质。
- `internal/codegen/ruleplan/ruleplan.go:187-198`、`../servora/cmd/internal/ruleplan/ruleplan.go:186-198`、`../servora/core/crud/lifecycle.go:140-147`：保留现有排序与预分配结构。仅为风格替换排序 API 会扩大修改，并可能在比较器传参时复制更大的结构值，不是这次必要优化。
- 不为使用 Go 1.27 泛型方法迁移公开函数/API，不强制扁平化嵌入结构字面量。
- 不把 `var x T; json.Unmarshal(..., &x); return &x` 改成 `new(expr)`；这种对象需要先提供地址再被写入，变量有实际用途。
- 不改生成物、第三方代码、外部库的指针构造 helper，或字符串中的 Go fixture 代码来追求统一风格。

## 官方现代化工具

本机已执行 `go help fix`、`go tool fix help` 与 `go tool fix help newexpr/errorsastype/embedlit`：

- `go fix -diff` 只打印补丁，不改源文件；有 diff 时退出码非零，应区分于加载/编译失败。
- 可通过 `-newexpr`、`-errorsastype` 等分析器选择范围。
- `newexpr` 分析器可能给 helper 增加 `//go:fix inline` 并保留 wrapper。此次明确采用直接迁移调用并删除无用私有 helper，不保留过渡层。
- 不对整个仓库直接执行写入式 `go fix ./...`；仅预览选定手写包后进行精确修改。
- [go fix command](https://pkg.go.dev/cmd/go#hdr-Update_packages_to_use_new_APIs)。

## 已完成的实际语言探针

在 `/tmp` 创建一次性 Go 程序，使用 `GOWORK=off GOTOOLCHAIN=local go run` 运行，之后删除文件。结果：

```text
go1.27.3 smoke passed: new(expr) value/type/alias, AsType wrapped/joined/custom/nil errors, CutPrefix empty/prefix cases
```

覆盖：字符串取址值与独立性、显式 `*int32`、nil/普通/包装/Join/自定义 As 错误匹配，以及空串、仅 `//`、正常前缀、legacy path、普通文本的前缀处理。该结果证明所选语言用法；后续实际项目构建、测试及运行结果见 `../implement.md`。

## 调研阶段工具状态（实施结果见 implement.md）

- GitNexus 1.6.12 可执行，但 `gitnexus list` 显示没有索引，不能声称已完成图影响分析。
- `gitnexus analyze --help` 已确认 `--index-only` 会跳过 AGENTS/CLAUDE/skills 注入。实施前为两个主要仓库建立索引，并对要改的函数/测试 helper 做 upstream impact；HIGH/CRITICAL 必须先说明风险，UNKNOWN 必须进一步确认。
- 主代理查询 `xd://lsp`：`gopls (configured, not started)`。父级工作区当前版本过低，先完成版本声明修改，再用 gopls references 获取调用点。
