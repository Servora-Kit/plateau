# ClickHouse 可选连接

适用于平台连接构造与应用装配。查询、审计表结构和数据业务归具体服务；本包只拥有连接配置、TLS 和连接建立。

[NewConnOptional](../../../../infra/clickhouse/clickhouse.go) 的返回语义必须区分：

| 返回 | 含义与调用方责任 |
| --- | --- |
| nil, nil | 没有配置或没有 addrs，消费方决定禁用该功能 |
| nil, err | 配置存在但校验／连接／Ping 失败，消费方明确失败启动或降级 |
| conn, nil | 已连接，消费方负责 Close |

函数克隆 Proto 配置后 Apply，映射连接池与超时；compression 仅支持空/none、lz4、zstd。TLS 使用 Servora TLS 能力构造。Ping 使用 dial timeout 派生的 context；Ping 失败主动关闭连接，成功后的生命周期交给 data/bootstrap。

不能把“未配置”和“配置失败”写成同一个 nil 分支；不能在 helper 与业务层重复记录相同日志。日志由消费方边界决定。

检查入口：`go test ./infra/clickhouse`；[测试](../../../../infra/clickhouse/clickhouse_test.go) 覆盖未配置、默认值、Ping 失败、TLS 与压缩错误、输入隔离，不代替实际 ClickHouse 端到端验收。配置源：[config.proto](../../../../api/protos/plateau/infra/clickhouse/v1/config.proto)。
