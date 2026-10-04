# 前端编译集成边界

- 使用 Next.js App Router 从 `@plateau/api` 导入真实生成符号，以无副作用、可渲染引用参与编译；不复制契约或实现 fake transport。
- workspace 生成包通过 `transpilePackages` 接入；此入口不引入请求 adapter、代理或外部服务调用，业务交互归对应应用。
- 构建检查只证明依赖可参与编译，不替代请求行为、服务可达性或业务验收。
