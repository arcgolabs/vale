# Vela 技术选型稳定版

## 1. 文档说明

本文档用于确定 Vela 的第一版技术选型。

Vela 暂定定位为：

> 一个轻量、可嵌入、基于 Provider 驱动、支持私有插件 Registry 与 RPC 插件体系的动态应用网关。

本文档重点围绕以下目标展开：

- 使用 Go 生态中成熟、稳定、可长期维护的组件；
- 尽可能复用已有生态，而不是无意义重复造轮子；
- 适度复用 arcgolabs 生态；
- 最大化运行时性能；
- 保持核心数据面足够轻量；
- 控制面保持动态、可扩展、可集群化；
- 插件系统支持私有 Registry 与 RPC 隔离。

---

## 2. 总体选型结论

Vela 的技术路线建议定为：

```text
语言：Go 1.26.x+
核心数据面：net/http + httputil.ReverseProxy + 自研 compiled runtime
配置：HCL v2 + fsnotify
Provider：自研接口 + File/Docker/Orch 内置
插件：HashiCorp go-plugin + gRPC + Protobuf
插件分发：File/HTTP/OCI Registry，OCI 使用 ORAS
集群：HashiCorp Raft，后置阶段实现
可观测：Prometheus + OpenTelemetry + slog
TLS：静态证书起步，CertMagic adapter 后置
arcgolabs：复用 logx/configx/collectionx/observabilityx/httpx，但不污染 runtime core
```

核心原则：

> Vela 的核心 runtime 要像标准库一样轻，控制面要像 Traefik 一样动态，插件体系要像 HashiCorp 一样隔离，分发体系要像 OCI 一样私有可控。

---

## 3. 基础语言与工程

| 方向 | 选型 | 结论 |
|---|---|---|
| 语言 | Go | 定稿 |
| Go 版本 | Go 1.26.x+ | 定稿 |
| 构建 | Go Modules + Taskfile | 定稿 |
| CLI | Cobra | 定稿 |
| 配置格式 | HCL 优先，YAML/JSON 后置 | 定稿 |
| 日志 | log/slog + arcgolabs/logx | 定稿 |
| 单元测试 | Go testing + testify 可选 | 稳定 |
| Benchmark | Go benchmark + benchstat | 定稿 |

建议 `go.mod`：

```go
go 1.26
```

开发和 CI 基线建议：

```text
Go >= 1.26.2
```

### 3.1 为什么选择 Go

Vela 属于网络代理和控制面系统，Go 的优势比较明显：

- 单二进制部署友好；
- 标准库网络能力成熟；
- 并发模型适合代理与控制面；
- 与 HashiCorp go-plugin、Raft、HCL 等生态契合；
- 对私有化部署、裸机部署、容器化部署都比较友好；
- 后续接入 OCI、Docker、Prometheus、OpenTelemetry 等生态成本低。

---

## 4. 数据面 Runtime

| 模块 | 选型 | 说明 |
|---|---|---|
| HTTP Server | net/http | 第一阶段定稿 |
| Reverse Proxy | oxy | 核心内置，默认唯一 engine |
| Transport | 自定义 http.Transport 池 | 定稿 |
| Snapshot | atomic.Pointer[*CompiledSnapshot] | 定稿 |
| Router | 自研编译型 matcher | 定稿 |
| LB | 自研 picker | 定稿 |
| Health Check | 自研 | 定稿 |
| WebSocket/SSE | 基于标准库代理能力 | 定稿 |

### 4.1 不建议第一版使用 fasthttp

第一版不建议用 fasthttp。

原因：

```text
协议正确性 > 连接复用 > 可观测性 > 低分配 > 极限 QPS
```

Vela 是网关，不是单纯 benchmark 项目。网关必须优先保证：

- WebSocket；
- SSE；
- Streaming；
- Header 透传；
- X-Forwarded-For；
- HTTP/2；
- Timeout；
- KeepAlive；
- Connection draining；
- 标准 HTTP 行为。

Go 标准库在这些方面的长期稳定性更强。

### 4.2 Runtime 核心结构

```go
type Runtime struct {
    current atomic.Pointer[CompiledSnapshot]
}
```

请求路径：

```text
request
  -> snapshot := current.Load()
  -> match entrypoint
  -> match router
  -> execute builtin middlewares
  -> optional rpc middleware
  -> choose upstream
  -> reverse proxy
  -> access log
  -> metrics
```

请求路径绝对不能访问：

- Provider；
- Plugin Registry；
- Raft；
- 数据库；
- 配置中心；
- Docker API；
- Orch registry；
- 远程控制面。

### 4.3 Runtime 边界

`runtime` 包只依赖：

```text
stdlib + oxy reverse proxy
config.CompiledSnapshot
router.Matcher
proxy.Proxy
lb.Picker
middleware.CompiledChain
accesslog.EventEmitter
metrics.Recorder
```

不依赖：

```text
hcl
docker
raft
plugin
registry
fsnotify
admin api
database
```

这条边界很关键。  
核心 runtime 必须保持 dependency-light。

---

## 5. Router 技术选型

Vela 不建议直接使用 chi、httprouter、gin 的 router。

原因是 Vela 的路由模型不是普通 Web API 路由，而是网关规则：

```text
Host(`api.example.com`) && PathPrefix(`/v1`) && Header(`X-Tenant`, `a`)
```

因此建议自研一个 gateway rule compiler。

### 5.1 Router 设计

| 层级 | 实现 |
|---|---|
| Rule Parser | 自研小型 parser |
| Rule AST | 自研 |
| Compile | 编译成 matcher chain |
| Host 匹配 | map[string]*HostRouter |
| Wildcard Host | suffix tree / 简化 wildcard map |
| PathPrefix | radix tree / prefix tree |
| Header/Method | 编译成 predicate |
| Priority | 编译期排序 |

### 5.2 推荐运行结构

```text
CompiledSnapshot
  -> EntrypointRouter
      -> HostRouter
          -> PathPrefixTree
              -> RouteCandidate[]
                  -> PredicateChain
```

请求时：

```text
entrypoint lookup
  -> host exact match
  -> wildcard host match
  -> path longest prefix
  -> priority sort result
  -> predicate check
  -> route selected
```

### 5.3 arcgolabs 复用

可以复用：

```text
arcgolabs/collectionx
```

尤其是：

- trie；
- prefix tree；
- mapping；
- set；
- immutable/snapshot 辅助结构。

如果当前 collectionx 的实现还没有完全适配网关路由，可以先在 Vela 内部写极简 radix tree，后续再沉淀回 arcgolabs。

### 5.4 不建议

```text
不建议用 CEL 作为主路由规则引擎
不建议用 expr-lang 作为主路由规则引擎
不建议用正则作为主路由模型
不建议每次请求解释 rule 字符串
```

Rule 必须在配置更新时编译，请求时只执行已编译 matcher。

---

## 6. 配置系统

| 方向 | 选型 |
|---|---|
| 主配置语言 | HCL v2 |
| 解析库 | github.com/hashicorp/hcl/v2 |
| 解码 | gohcl + 自定义 schema validate |
| 文件监听 | fsnotify |
| 环境变量覆盖 | arcgolabs/configx |
| 配置合并 | 自研 Aggregator |
| 配置校验 | 自研 Validator |

### 6.1 为什么选择 HCL

Vela 的配置天然适合 HCL：

- entrypoint；
- router；
- service；
- middleware；
- plugin；
- provider；
- tls；
- health check；
- cluster。

相比 YAML，HCL 对复杂结构和人类编辑更友好。

### 6.2 配置流

```text
file / docker / orch / plugin provider
  -> PartialSnapshot
  -> Aggregator
  -> Full Snapshot
  -> Validator
  -> Compiler
  -> atomic swap
```

### 6.3 不建议使用 Viper 作为核心配置层

Viper 适合普通应用配置，但 Vela 需要：

- 多 Provider 合并；
- 局部配置；
- 动态配置事件；
- 快照编译；
- 错误回滚；
- 配置来源追踪；
- 多版本快照。

这些能力应该由 Vela 自己控制。

---

## 7. Provider 体系

| Provider | 选型 |
|---|---|
| File Provider | HCL + fsnotify |
| Static Provider | 内置 |
| Orch Provider | 原生实现 |
| Docker Provider | Docker 官方 Go SDK |
| Swarm Provider | Docker 官方 Go SDK |
| Consul Provider | 后置，优先插件 |
| Nacos Provider | 后置，优先插件 |
| Etcd Provider | 后置，优先插件 |

### 7.1 Provider 接口

```go
type Provider interface {
    Name() string
    Watch(ctx context.Context, emit func(ProviderEvent)) error
}
```

Provider 只输出事件：

```go
type ProviderEvent struct {
    Source   string
    Revision string
    Snapshot *PartialSnapshot
}
```

Provider 不允许直接修改 Runtime。

### 7.2 Provider 原则

Provider 只做三件事：

```text
发现配置
转换配置
发出事件
```

不做：

```text
路由匹配
负载均衡
健康检查
请求转发
插件调度
Runtime apply
```

### 7.3 第一阶段 Provider

第一阶段建议内置：

```text
File Provider
Static Provider
```

第二阶段：

```text
Orch Provider
Docker Provider
Swarm Provider
```

第三阶段：

```text
Provider Plugin
```

---

## 8. Plugin 技术选型

| 方向 | 选型 |
|---|---|
| 插件进程模型 | HashiCorp go-plugin |
| RPC 协议 | gRPC |
| IDL | Protobuf |
| 代码生成 | buf + protoc-gen-go |
| 插件分发 | OCI Registry 优先 |
| OCI 客户端 | ORAS Go |
| HTTP/File Registry | 自研轻量实现 |
| 签名校验 | Sigstore/cosign 后置 |
| 插件运行 | 独立进程 + supervisor |

### 8.1 为什么使用 RPC 插件

RPC 插件相比源码解释插件更适合 Vela：

- 插件是预编译二进制；
- 不依赖 GitHub；
- 可以从私有 Registry 分发；
- 插件崩溃不直接拖垮宿主；
- 插件可以独立升级；
- 插件可以多语言实现；
- 宿主可以控制插件能力、超时和错误策略。

### 8.2 插件类型优先级

建议实现顺序：

```text
1. Provider Plugin
2. AccessLog Sink Plugin
3. Middleware Decision Plugin
4. Certificate Plugin
```

### 8.3 Middleware Plugin 限制

第一版 Middleware Plugin 只允许：

```text
读取 method/host/path/header/remote_addr
返回 continue/reject/redirect
设置 header
删除 header
返回简单错误响应
```

第一版不允许：

```text
读取完整 body
修改 streaming body
接管 ResponseWriter
接管完整 upstream proxy
执行长时间请求处理
```

原因：

请求 body 一旦进入插件，复杂度会明显增加，包括：

- 流式处理；
- 内存占用；
- 超时；
- 压缩；
- chunked encoding；
- body replay；
- back pressure。

### 8.4 插件调用策略

Middleware Plugin 必须支持：

```text
timeout
on_error
capabilities
max_concurrency
```

示例：

```hcl
middleware "corp-authz" {
  type     = "plugin"
  plugin   = "corp/authz"
  version  = "1.0.0"
  timeout  = "50ms"
  on_error = "reject"
}
```

认证、鉴权类插件默认：

```text
fail-closed
```

日志、审计类插件默认：

```text
fail-open
```

---

## 9. Plugin Registry 与插件包

| 方向 | 选型 |
|---|---|
| 默认 Registry | File Registry |
| 私有网络 Registry | HTTP Registry |
| 生产推荐 Registry | OCI Registry |
| 插件 manifest | HCL |
| checksum | SHA256 |
| signature | 后续接 Sigstore/cosign |
| 插件缓存 | 本地 content-addressable store |

### 9.1 插件包结构

```text
corp-authz/
  manifest.hcl
  checksums.txt
  signatures/
  linux-amd64/plugin
  linux-arm64/plugin
  darwin-arm64/plugin
  windows-amd64/plugin.exe
```

### 9.2 manifest 示例

```hcl
plugin "authz" {
  namespace   = "corp"
  name        = "authz"
  version     = "1.0.0"
  type        = "middleware"
  protocol    = "grpc"
  api_version = "vela.plugin.v1"

  capabilities = [
    "request.headers.read",
    "request.headers.write",
    "response.reject"
  ]

  binary {
    os     = "linux"
    arch   = "amd64"
    path   = "linux-amd64/plugin"
    sha256 = "..."
  }
}
```

### 9.3 Registry 实现顺序

```text
Phase 1: File Registry
Phase 2: HTTP Registry
Phase 3: OCI Registry
```

### 9.4 插件安装命令

```bash
vela plugin install file:///opt/vela/plugins/corp-authz
vela plugin install https://registry.example.com/vela/plugins/corp-authz/1.0.0
vela plugin install oci://registry.example.com/vela/plugins/authz:1.0.0
```

---

## 10. 集群技术选型

| 方向 | 选型 |
|---|---|
| 一致性 | HashiCorp Raft |
| 成员发现 | memberlist 后置 |
| Manager/Proxy | 自研 |
| Snapshot 分发 | gRPC stream |
| 状态存储 | Raft FSM + 本地 WAL |
| 请求路径 | 只读本地快照 |
| 证书同步 | 后置 |

### 10.1 集群不进入第一阶段

集群能力建议放在第三或第四阶段。

原因：

- 单机 Runtime 必须先稳定；
- Provider + Snapshot 模型必须先稳定；
- 插件模型必须先稳定；
- 否则 Raft 会把系统复杂度提前放大。

### 10.2 角色设计

```text
velad --role=manager,proxy
velad --role=manager
velad --role=proxy
```

Manager 节点负责：

```text
Raft
配置存储
Provider watch
Snapshot build
插件元信息管理
证书元信息管理
集群成员管理
```

Proxy 节点负责：

```text
请求转发
本地快照消费
健康检查
访问日志
metrics
```

### 10.3 Raft 同步内容

同步：

```text
静态配置
Provider 元信息
插件安装元信息
证书元信息
集群成员
Snapshot 版本
```

不同步：

```text
请求日志
每次健康检查结果
每次请求状态
连接状态
访问日志明细
```

请求路径永远不能读 Raft。

---

## 11. TLS / 证书

| 阶段 | 选型 |
|---|---|
| Phase 1 | 静态证书 + 文件热加载 |
| Phase 2 | Certificate Provider |
| Phase 3 | CertMagic adapter |
| Phase 4 | 集群证书同步 |

### 11.1 第一阶段

第一阶段只做：

```text
静态证书
证书文件加载
证书热更新
SNI 基础匹配
```

### 11.2 后续阶段

后续支持：

```text
ACME
CertMagic adapter
证书插件
证书 Registry
集群证书同步
```

### 11.3 设计原则

CertMagic 可以作为 adapter，但不要污染核心 TLS 抽象。

核心接口：

```go
type CertificateProvider interface {
    GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}
```

---

## 12. 可观测性

| 方向 | 选型 |
|---|---|
| Metrics | Prometheus client_golang |
| Tracing | OpenTelemetry Go |
| Runtime Log | slog + arcgolabs/logx |
| Access Log | 自研结构化事件 |
| Audit Log | 自研事件模型 |
| Dashboard | 后置 |

### 12.1 日志分类

Vela 日志分三类：

```text
Runtime Log
Access Log
Audit Log
```

### 12.2 Runtime Log

用于记录：

- 启动；
- 停止；
- 配置加载；
- 快照切换；
- 插件启动；
- Provider 错误；
- 健康检查状态变化。

### 12.3 Access Log

访问日志不要直接当普通日志打。

建议定义事件：

```go
type AccessEvent struct {
    Time        time.Time
    RequestID   string
    TraceID     string
    Method      string
    Host        string
    Path        string
    Status      int
    Duration    time.Duration
    Router      string
    Service     string
    Upstream    string
    BytesIn     int64
    BytesOut    int64
    Error       string
}
```

Access Log 输出方式：

```text
stdout JSON
file JSON
plugin batch sink
```

### 12.4 Metrics 指标

第一版必须有：

```text
vela_requests_total
vela_request_duration_seconds
vela_upstream_duration_seconds
vela_upstream_errors_total
vela_routes_total
vela_services_total
vela_endpoints_total
vela_healthy_endpoints_total
vela_snapshot_version
vela_snapshot_apply_total
vela_snapshot_apply_errors_total
vela_plugin_status
vela_plugin_rpc_duration_seconds
```

---

## 13. 压缩与响应处理

| 方向 | 选型 |
|---|---|
| gzip | 标准库 gzip 起步 |
| 高性能 gzip/zstd | klauspost/compress 后置 |
| brotli | 后置 |
| response buffer | 尽量不缓存 body |
| streaming | 直接透传 |

第一版建议只做：

```text
compress middleware: gzip
```

并默认关闭，由用户显式打开。

### 13.1 原则

- 不默认缓存响应 body；
- 不破坏 streaming；
- 不影响 SSE；
- 不对 WebSocket 做压缩中间件；
- 大响应优先透传。

---

## 14. arcgolabs 生态复用建议

### 14.1 应该复用

| arcgolabs 模块 | 用途 | 建议 |
|---|---|---|
| logx | slog 适配、结构化日志 | 复用 |
| configx | env/bootstrap 配置 | 复用 |
| collectionx | trie、mapping、set 等基础结构 | 可复用 |
| observabilityx | OTel/metrics 封装 | 可复用 |
| eventx | 控制面事件分发 | 可复用但不要进请求路径 |
| httpx | Admin API | 可复用 |
| clientx | 插件 registry / admin client | 可复用 |
| authx | Admin API auth / 示例插件 | 后置复用 |

### 14.2 不建议直接复用到核心热路径

| 模块 | 原因 |
|---|---|
| httpx | 适合 Admin API，不适合代理数据面 |
| eventx | 控制面可以用，请求热路径不要走 event bus |
| dix/fx | app assembly 可用，core runtime 不要依赖 DI |
| dbx/sqltmplx | 第一阶段网关 core 不需要 DB |
| bunx | 第一阶段网关 core 不需要 ORM |

### 14.3 arcgolabs 复用边界

建议原则：

```text
Vela core 尽量 dependency-light
Vela `cmd/velad`（进程入口）可以复用 arcgolabs
Vela provider/plugin/admin 可以更多复用 arcgolabs
```

不要让公共库反向决定网关核心架构。

---

## 15. 最终依赖清单

### 15.1 Core Runtime

```text
stdlib + oxy reverse proxy:
  net/http
  net
  crypto/tls
  sync/atomic
  context
  log/slog
```

原则：

```text
core runtime 以标准库为主，反向代理固定使用 oxy
```

### 15.2 Config

```text
github.com/hashicorp/hcl/v2
github.com/hashicorp/hcl/v2/gohcl
github.com/fsnotify/fsnotify
arcgolabs/configx
```

### 15.3 Provider

```text
github.com/docker/docker/client
github.com/docker/docker/api/types/...
```

Orch Provider 使用内部接口，不建议第一天做成外部插件。

### 15.4 Plugin

```text
github.com/hashicorp/go-plugin
google.golang.org/grpc
google.golang.org/protobuf
buf.build tooling
oras.land/oras-go/v2
```

### 15.5 Cluster

```text
github.com/hashicorp/raft
github.com/hashicorp/raft-boltdb/v2
```

Raft storage 可以先用 bbolt adapter。  
后续如果你的 bbolt/badger 封装稳定，可以再替换。

### 15.6 Observability

```text
github.com/prometheus/client_golang/prometheus
github.com/prometheus/client_golang/prometheus/promhttp
go.opentelemetry.io/otel
arcgolabs/observabilityx
```

### 15.7 TLS 后置

```text
github.com/caddyserver/certmagic
```

只做 adapter，不进入核心抽象。

### 15.8 Compression 后置

```text
github.com/klauspost/compress
```

---

## 16. 不建议选型

### 16.1 不建议 fasthttp 作为第一版核心

原因：

- 标准 HTTP 行为需要额外适配；
- WebSocket/SSE/Streaming 场景更容易踩坑；
- Go 标准库生态更稳；
- 第一版应优先正确性和可维护性。

### 16.2 不建议 Yaegi / Go 源码解释插件

原因：

- 你明确不喜欢 Traefik 的插件模型；
- 源码解释不适合私有二进制分发；
- 不利于插件隔离；
- 不利于多语言扩展。

### 16.3 不建议 Go 标准库 plugin

原因：

- 平台限制明显；
- `.so` 分发复杂；
- 插件和宿主 Go 版本、依赖一致性要求高；
- 不适合独立进程隔离。

### 16.4 不建议把 Consul / Nacos / Etcd SDK 放进核心

这些应该作为 Provider 或 Plugin。

核心只认 Snapshot，不关心配置从哪里来。

### 16.5 不建议第一版做 ACME

先做：

```text
静态证书
文件热加载
SNI
证书 Provider 抽象
```

ACME 后面通过 CertMagic adapter 实现。

### 16.6 不建议请求热路径走复杂 RPC 插件链

RPC 插件适合：

```text
Provider
AccessLog
Authz 决策
Certificate
Audit
```

不适合：

```text
每个请求串多个 RPC 中间件
处理完整 body
接管 ResponseWriter
```

---

## 17. 性能设计原则

### 17.1 配置期编译，运行期只读

```text
HCL / Provider Event
  -> AST
  -> Snapshot
  -> CompiledSnapshot
  -> atomic swap
```

请求期不能解释配置。

### 17.2 Router 编译成结构

```text
host map
  -> path radix tree
  -> sorted route candidates
  -> predicate chain
```

不要每次请求遍历所有 routes。

### 17.3 Middleware Chain 预编译

配置更新时生成：

```go
type HandlerChain func(http.ResponseWriter, *http.Request)
```

请求时直接执行，不要动态查 map。

### 17.4 LB Picker 无锁或低锁

Round-robin 用 atomic counter：

```go
idx := atomic.AddUint64(&counter, 1) % uint64(len(endpoints))
```

健康 endpoint 使用快照切换，不要请求时抢锁。

### 17.5 Access Log 异步

请求结束只写入 channel/ring buffer。

日志落盘、JSON encode、插件 sink 不要阻塞请求主路径。

### 17.6 插件调用必须限流和超时

建议：

```text
authz timeout: 20ms ~ 50ms
accesslog plugin: async batch
provider plugin: control plane only
```

### 17.7 ReverseProxy 使用 BufferPool

`httputil.ReverseProxy` 支持 BufferPool，可以通过复用临时 byte slice 降低 copy buffer 分配。

### 17.8 避免热路径 interface 过度抽象

核心 hot path 中少用深层 interface 链，尤其是：

- 路由匹配；
- LB pick；
- middleware 执行；
- access log event 构造。

可以在配置编译阶段把动态性解决掉，请求时执行具体函数。

---

## 18. 推荐仓库结构

```text
vela/
  cmd/
    vela/
    velad/

  pkg/
    gateway/              # embedded API
    runtime/              # core runtime, oxy proxy engine
    router/               # rule parser + matcher compiler
    proxy/                # reverse proxy wrapper
    lb/                   # picker
    health/               # health checker
    config/               # snapshot model + validator
    provider/             # provider interface
      file/
      docker/
      orch/
      static/
    middleware/           # builtin middleware
    accesslog/            # event + sinks
    metrics/              # prometheus
    tlsx/                 # static cert first, certmagic adapter later
    plugin/               # plugin manager
      protocol/
      registry/
      installer/
      supervisor/
    cluster/              # later
    admin/                # admin API
```

关键边界：

```text
runtime 不依赖 plugin
runtime 不依赖 provider
runtime 不依赖 cluster
runtime 不依赖 docker
runtime 不依赖 raft
runtime 不依赖 HCL
```

runtime 只消费：

```go
*CompiledSnapshot
```

---

## 19. 第一版实现顺序

### Step 1：核心 Runtime

- Snapshot 模型；
- CompiledSnapshot；
- atomic swap；
- route matcher；
- reverse proxy；
- upstream picker。

### Step 2：File Provider

- HCL parser；
- config validator；
- file watch；
- reload；
- invalid config rollback。

### Step 3：基础网关能力

- entrypoint；
- router；
- service；
- endpoint；
- round-robin；
- weighted round-robin；
- health check。

### Step 4：可观测性

- runtime log；
- access log；
- metrics；
- admin API。

### Step 5：Embedded API

- gateway.New；
- Start/Stop；
- Provider 注入；
- Logger 注入；
- Metrics 注入；
- 生命周期管理。

### Step 6：插件系统最小闭环

- manifest；
- local plugin；
- process launcher；
- handshake；
- provider plugin demo；
- middleware plugin demo。

### Step 7：Orch Provider

- service watch；
- endpoint watch；
- health status；
- route generation；
- snapshot update。

---

## 20. MVP 验收标准

第一阶段 MVP 应满足：

1. 可以通过 HCL 配置启动 `velad`；
2. 支持 HTTP entrypoint；
3. 支持 HTTPS 静态证书；
4. 支持 Host 路由；
5. 支持 PathPrefix 路由；
6. 支持 Header / Method predicate；
7. 支持多个 upstream endpoint；
8. 支持 Round Robin；
9. 支持 Weighted Round Robin；
10. 支持 HTTP health check；
11. 支持配置热更新；
12. 配置错误时保留旧快照；
13. 支持 JSON access log；
14. 支持 Prometheus metrics；
15. 支持 Admin API 查看当前 routes/services/endpoints；
16. 支持 WebSocket 基础代理；
17. 支持 SSE 基础代理；
18. 支持 Go SDK 嵌入启动最小网关实例。

---

## 21. 版本演进建议

### Phase 1：单机动态网关

目标：做出稳定单机版本。

功能：

```text
velad
File Provider
HCL
HTTP/HTTPS reverse proxy
Host/PathPrefix routing
Round Robin
Weighted Round Robin
Health Check
Hot Reload
Access Log
Prometheus Metrics
Admin API
```

### Phase 2：Embedded + Orch

目标：成为 orch 默认入口运行时。

功能：

```text
Embedded API
Orch Provider
服务实例动态发现
endpoint 动态更新
route snapshot 热切换
请求链路指标接入 orch
```

### Phase 3：插件体系

目标：实现私有插件生态。

功能：

```text
Plugin Manifest
File Registry
HTTP Registry
OCI Registry
Plugin Installer
Plugin Verifier
Plugin Supervisor
Provider Plugin
Middleware Plugin
AccessLog Sink Plugin
```

### Phase 4：集群模式

目标：实现轻量集群网关。

功能：

```text
Manager / Proxy 角色
Raft 控制面
Snapshot 分发
节点注册
集群成员管理
插件状态同步
证书状态同步
```

### Phase 5：生产增强

目标：增强生产使用能力。

功能：

```text
ACME
Canary
Traffic Mirroring
Rate Limit
Circuit Breaker
Retry
Request Timeout Policy
Advanced TLS
Dashboard
```

---

## 22. 最终结论

Vela 的稳定技术选型应遵循：

```text
核心 runtime 极简
控制面动态扩展
插件进程隔离
Provider 一等公民
数据面本地只读
配置期编译
请求期无远程依赖
私有 Registry 优先
arcgolabs 适度复用
```

最终建议：

```text
语言：Go 1.26.x+
HTTP Runtime：net/http + httputil.ReverseProxy
Router：自研 compiled matcher
配置：HCL v2
Provider：自研接口
插件：HashiCorp go-plugin + gRPC
插件分发：OCI / HTTP / File Registry
集群：HashiCorp Raft，后置
可观测：Prometheus + OpenTelemetry + slog
TLS：静态证书起步，CertMagic adapter 后置
arcgolabs：复用外围能力，不污染 core runtime
```

这套选型既能最大化复用 Go 成熟生态，又保留 Vela 自己的核心控制力。  
真正要自研的是动态网关最关键的部分：Snapshot、Router、Provider 聚合、Runtime 编译、插件管控和嵌入式生命周期。  
不该自研的是 HTTP 协议栈、RPC 协议、Raft、OCI 分发、指标协议这些成熟底层能力。
