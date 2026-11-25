# 大型云平台与多服务 SaaS 统一审计架构调研

## 调研问题

在包含身份底座（`iam`）、平台管理台（`admin`）、审计服务（`audit`）及后续可扩展业务微服务（如 `cms`、`mall`、`saas`）的微服务架构中：
1. 业务微服务的审计事件是否应当写入平台共享的 `audit.service`？
2. 平台管理台 `app/admin` 能否在不耦合业务领域逻辑的前提下直接呈现业务微服务的审计记录？
3. 哪些业务数据应当进入平台审计服务，哪些必须留在业务自身数据库？

---

## 一手参考来源与核心设计模式

### 1. Google Cloud Audit Logs（统一审计管道 + 四类标准日志分类）

- **来源**：[Google Cloud Audit Logs Overview](https://docs.cloud.google.com/logging/docs/audit)
- **核心做法**：
  - 全平台所有服务（底层 IAM 以及上层 Compute、Firestore、BigQuery 等百余个产品服务）统一按相同的 `AuditLog` 标准结构写入中央 Cloud Logging，通过 `serviceName`（如 `iam.googleapis.com`、`firestore.googleapis.com`）、`methodName`、`resourceName`、`authenticationInfo` 标识来源，不在每个产品线重复搭建独立审计存储。
  - 严格按性质切分为四类：
    1. **Admin Activity（管理与配置变更）**：所有改变资源状态或配置的写操作，**默认强制开启**且免费留存 400 天；
    2. **Policy Denied（策略与鉴权拒绝）**：越权或安全策略拦截记录；
    3. **System Event（系统自动动作）**：非人类触发的平台生命周期事件；
    4. **Data Access（数据面读写访问）**：因体量极大，**默认关闭**（除 BigQuery 外需显式开启并独立计费，且需要更高权限角色 `roles/logging.privateLogViewer` 才可查阅）。
  - 组织与项目级安全管理员在统一的 **Logs Explorer** 中按服务名、资源类型、主体与时间窗口跨服务检索。

### 2. AWS CloudTrail Lake & AWS Security Reference Architecture / SaaS Lens 白皮书

- **来源**：
  - [AWS Security Reference Architecture (AWS SRA)](https://docs.aws.amazon.com/pdfs/prescriptive-guidance/latest/security-reference-architecture/security-reference-architecture.pdf)
  - [AWS Well-Architected SaaS Lens Whitepaper](https://docs.aws.amazon.com/pdfs/wellarchitected/latest/saas-lens/wellarchitected-saas-lens.pdf)
  - [AWS CloudTrail Lake Multi-Source Ingestion](https://aws.amazon.com/blogs/mt/ingesting-activity-events-from-non-AWS-sources-to-aws-cloudtrail-lake/)
  - [AWS AppFabric OCSF Cross-Application Audit](https://aws.amazon.com/blogs/business-productivity/cross-application-audit-log-analysis-with-aws-appfabric/)
- **核心做法**：
  - **独立安全归集域**：AWS SRA 将所有账号与所有服务的审计事件集中投递至独立的审计存储域（Log Archive），使业务服务即使发生数据库误删或被攻破，问责证据依然独立留存。
  - **标准化信封解耦业务语义**：CloudTrail Lake 与 AppFabric（采用 OCSF 开放安全模型）要求各业务应用在生产端将关键动作归一化为统一的「主体（Actor）+ 动作（Operation）+ 目标资源（Resource）+ 结果（Outcome）+ 扩展 JSON 载荷」，中央审计存储与查询控制台只消费标准信封，不感知具体业务表结构。
  - **平台管控面与多租户应用面隔离（SaaS Lens）**：平台运营方在中央管控台查看全局跨服务审计；若上层存在多租户 SaaS 应用需要向“租户自己的管理员”开放审计查询，则必须在服务端通过已认证的 `tenant_id` 强制过滤，禁止把平台全局管控台直接暴露给外部租户。

### 3. 阿里云操作审计（ActionTrail）与《阿里云安全白皮书（2024）》

- **来源**：
  - [什么是操作审计 ActionTrail](https://help.aliyun.com/zh/actiontrail/product-overview/what-is-actiontrail)
  - [多账号操作日志统一归集与审计](https://help.aliyun.com/zh/actiontrail/)
  - [阿里云安全白皮书（2024 版）](https://security-help-center.oss-cn-hangzhou.aliyuncs.com/%E9%98%BF%E9%87%8C%E4%BA%91%E5%AE%89%E5%85%A8%E7%99%BD%E7%9A%AE%E4%B9%A6%EF%BC%882024%E7%89%88%EF%BC%89.pdf)
- **核心做法**：
  - 统一汇聚 RAM（身份）与 ECS、OSS、RDS、ACK 等各产品线的**管控事件**，集中留存 180 天以上以满足等保合规，并支持跨服务关联排查异常登录、密钥泄露与非计划变更。

### 4. WorkOS Audit Logs（面向多业务 SaaS 的共享审计服务）

- **来源**：[WorkOS Developer's Guide to Audit Logs / SIEM](https://workos.com/blog/the-developers-guide-to-audit-logs-siem) · [WorkOS Log Streams](https://workos.com/docs/audit-logs/log-streams)
- **核心做法**：
  - 明确区分**合规问责事件**（登录、权限变更、配置修改、敏感导出、关键资源增删改）与**应用性能/调试日志或领域业务历史**（内部报错堆栈、性能指标、商品浏览流水不进入审计库）。

---

## 对 Plateau 架构的映射结论

1. **为什么 `admin` 不能塞业务管理页，但 `audit` 可以接业务审计并在 `admin` 呈现？**
   - 业务管理操作（如 CMS 富文本排版、Mall 订单履约发货）强依赖各自的领域状态机与定制交互，放进 `app/admin` 会破坏微服务自治；
   - 而**审计问责记录是天然同构的只读事实**。Plateau 现行 CloudEvents 信封（`service`、`category`、`operation`、`actor_type/id`、`resource_type/name`、`success/error`、`trace_id`、`detail_json`）对 `//cms.service`、`//mall.service` 与 `//iam.service` 完全通用——`audit.service` 和 `app/admin` 审计页无需引入任何 CMS / Mall 业务代码，即可原样入库、筛选、展示脱敏 `detail_json` 并按 `trace_id` 串联跨服务调用链。

2. **业务微服务接入 `audit.service` 的三条准入准则**：
   - **准入范围（写什么）**：仅写入管理/运营关键状态变更、敏感配置修改与 403 授权拒绝（对应 GCP Admin Activity + Policy Denied）。
   - **排除范围（不写什么）**：业务领域自有历史数据（如商城订单物流状态表、CMS 文章历史版本正文，业务代码需实时读写它们，必须留在业务自己的 PostgreSQL）、C 端高频只读访问与埋点流水、大体积正文载荷（单条仅存脱敏摘要）。
   - **查看受众（谁来看）**：`app/admin` 仅面向平台超级管理员 / 安全合规人员呈现全平台跨服务审计；未来若某个多租户 `saas.service` 需要让“租户客户管理员”在租户后台自查本租户员工操作日志，届时在审计信封与 `audit.events` 扩展可选 `tenant_id` 列，由该 SaaS 后端强制注入 `tenant_id` 过滤条件只读调用 `audit.service`，不向租户开放 `app/admin`。

3. **对 `service` 筛选发现机制的影响**：
   - 一旦允许未来 `cms.service`、`mall.service` 动态接入且不希望每上新服务都改 `app/admin/web` 代码，单纯靠前端写死字典或仅从当前页 20 条记录里拼凑 `service` 就会漏掉低频业务服务。
   - Plateau 的 ClickHouse `audit.events` 表中 `service` 列已声明为 `LowCardinality(String)`（`app/audit/service/internal/data/schema/event.go:71`），执行无 `FINAL` 的 `SELECT DISTINCT service FROM events WHERE service != ''` 仅扫描 1 字节词典索引，千万行耗时在毫秒级；配合前端 3 个底座服务保底项即可同时兼顾冷启动与新业务微服务零前端改动的自动发现。
