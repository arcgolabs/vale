# Vale 产品设计文档

## 1. 产品概述

### 1.1 产品名称

暂定名称：**Vale**

> Vale 是一个轻量、可嵌入、基于 Provider 驱动的动态应用网关。

Vale 的目标不是简单复制 Traefik 或 Caddy，而是面向私有化部署、自研编排系统、裸机服务治理、Docker/Swarm 场景，提供一个同时支持 **独立二进制运行** 与 **嵌入式集成** 的轻量应用网关。

### 1.2 一句话定位

**Vale 是一个可嵌入、可独立部署、支持私有插件生态的轻量动态应用网关。**

### 1.3 核心关键词

- Lightweight Gateway
- Embedded Runtime
- Standalone Binary
- Provider-driven
- Snapshot-based Runtime
- Private Plugin Registry
- RPC Plugin System
- Dynamic Reverse Proxy
- Service Discovery
- Cluster-ready

---

## 2. 背景与问题

### 2.1 当前背景

在自研轻量编排系统 `orch` 的过程中，系统需要具备类似 Kubernetes Ingress / Traefik / Caddy 的入口流量能力，用于将外部请求路由到内部服务实例。

前期曾考虑直接嵌入 Caddy，或使用 Traefik 作为现成网关。但在实际设计中暴露出几个关键问题：

1. Caddy 的嵌入式模型与自身运行时、模块系统和日志体系耦合较深。
2. Traefik 的插件机制依赖官方插件目录和 GitHub 生态，不适合完全私有化插件分发。
3. Traefik 的集群化能力主要面向企业版本，不适合自研编排系统内置轻量集群入口能力。
4. 现有通用网关往往更关注 Kubernetes / Docker / 云原生标准场景，而不是自研编排系统、裸机和私有平台的组合场景。
5. 希望插件以预编译二进制 + RPC 的方式运行，而不是脚本解释执行或运行时加载源码。

因此，Vale 的目标是提供一个更贴合自研平台需求的轻量应用网关内核。

---

## 3. 产品目标

### 3.1 核心目标

Vale 的核心目标包括：

1. **可独立运行**  
   提供 `valed` 二进制，可作为独立网关部署。

2. **可嵌入运行**  
   提供 Go SDK，可嵌入 `orch` 或其他自研系统中作为入口流量运行时。

3. **Provider 驱动**  
   通过 Provider 接入不同配置来源和服务发现来源，例如 File、Docker、Swarm、orch、Consul、Nacos、Etcd 等。

4. **Snapshot 驱动的数据面**  
   控制面动态生成配置快照，数据面仅消费已编译快照，请求路径不依赖锁和远程状态。

5. **私有插件 Registry**  
   支持自建插件仓库，插件不依赖 GitHub 官方目录。

6. **RPC 插件机制**  
   插件以独立进程方式运行，通过 RPC/gRPC 与宿主通信，提升隔离性、可控性和语言扩展能力。

7. **集群能力可演进**  
   早期支持单机动态网关，中后期支持 Manager / Proxy 节点角色分离与 Raft 控制面状态同步。

### 3.2 非目标

Vale 第一阶段不追求成为完整 API Gateway 或 Service Mesh。

第一阶段不做：

- 完整服务网格能力
- 复杂租户计费
- 完整 API 管理平台
- 请求 body 级插件处理
- HTTP/3
- TCP/UDP 网关
- Kubernetes CRD Controller
- Wasm 插件平台
- 大规模多区域控制面
- 与 Envoy 同等级别的 L7 代理能力

Vale 的第一原则是：**先成为稳定、轻量、可控的入口流量运行时。**

---

## 4. 用户与使用场景

### 4.1 目标用户

1. **自研编排系统开发者**  
   需要为自研调度/编排系统提供内置 Ingress / Gateway 能力。

2. **私有化部署团队**  
   需要可控、轻量、支持私有插件分发的动态反向代理。

3. **中小型平台团队**  
   不希望引入完整 Kubernetes，但需要服务发现、动态路由、健康检查和入口流量治理。

4. **裸机 / Docker / Swarm 用户**  
   希望有一个比 Nginx 更动态、比 K8s Ingress 更轻量的入口网关。

5. **企业内部平台团队**  
   需要把认证、限流、审计、证书、日志等能力通过私有插件扩展。

### 4.2 典型使用场景

#### 场景一：作为 orch 的内置网关

`orch` 负责服务调度、实例生命周期和服务注册，Vale 作为嵌入式 Gateway Runtime 负责接收外部请求并转发到对应服务实例。

```text
orch controller / worker
        |
        | embedded gateway runtime
        v
      Vale
        |
        | route to service endpoints
        v
   service instances
```

#### 场景二：作为独立边缘网关

Vale 以 `valed` 的形式运行，通过 HCL/YAML 文件或 Docker/Swarm Provider 加载服务路由配置。

```text
Client
  |
  v
valed
  |
  +-- service-a
  +-- service-b
  +-- service-c
```

#### 场景三：私有插件网关

企业内部自建插件 Registry，开发认证、签名、限流、审计等插件，并由 Vale 动态加载。

```text
valed
  |
  +-- private plugin registry
  |       +-- authz plugin
  |       +-- rate-limit plugin
  |       +-- audit plugin
  |
  +-- runtime plugin supervisor
```

#### 场景四：轻量集群网关

多个 Vale 节点组成 Gateway Cluster，Manager 节点负责配置和状态同步，Proxy 节点负责流量转发。

```text
Manager Nodes
  - raft
  - config store
  - provider watch
  - snapshot build

Proxy Nodes
  - local runtime
  - reverse proxy
  - no remote dependency in request path
```

---

## 5. 产品原则

### 5.1 控制面动态，数据面本地

控制面可以动态发现服务、加载配置、合并 Provider 输出、管理插件和分发快照。

数据面必须尽量简单：

```text
request -> local compiled snapshot -> route match -> middleware -> load balance -> proxy
```

请求路径不能依赖：

- Raft 查询
- 远程数据库
- Provider 实时查询
- 插件 Registry
- 配置中心网络调用

### 5.2 Provider 是一等公民

Vale 不把配置文件作为唯一配置来源，而是将所有配置来源抽象为 Provider。

Provider 可以包括：

- File Provider
- Docker Provider
- Swarm Provider
- Orch Provider
- Consul Provider
- Nacos Provider
- Etcd Provider
- Static Provider
- Plugin Provider

### 5.3 插件隔离优先

插件默认以独立进程运行，通过 RPC/gRPC 与宿主通信。

这样可以实现：

- 插件崩溃不直接拖垮宿主
- 插件可以独立升级
- 插件可以使用不同语言实现
- 插件权限可以被约束
- 插件可以来自私有 Registry

### 5.4 热路径克制

不是所有能力都适合放进请求热路径。

控制面插件优先使用 RPC 插件；请求热路径插件必须控制调用次数、超时、错误策略和数据范围。

### 5.5 嵌入式和独立运行同等重要

Vale 从第一天就要同时支持：

- 独立二进制运行
- Go SDK 嵌入运行

不能把其中一种形态当作临时补丁。

---

## 6. 功能范围

## 6.1 第一阶段 MVP 功能

第一阶段目标是做出一个稳定可用的单机动态网关。

### 6.1.1 Listener / Entrypoint

支持配置多个入口监听器：

- HTTP
- HTTPS 静态证书
- 不同端口
- 不同绑定地址

示例：

```hcl
entrypoint "web" {
  address = ":80"
}

entrypoint "websecure" {
  address = ":443"
  tls = true
}
```

### 6.1.2 Router

支持基于规则的路由匹配：

- Host
- Path
- PathPrefix
- Header
- Method

示例：

```hcl
router "api" {
  entrypoints = ["web"]
  rule        = "Host(`api.example.com`) && PathPrefix(`/v1`)"
  service     = "api-service"
  middlewares = ["request-id", "compress"]
}
```

### 6.1.3 Service

支持服务和 endpoint 定义：

```hcl
service "api-service" {
  lb = "round_robin"

  endpoint {
    url = "http://10.0.0.11:8080"
  }

  endpoint {
    url = "http://10.0.0.12:8080"
  }
}
```

### 6.1.4 Load Balancing

第一阶段支持：

- Round Robin
- Weighted Round Robin
- Random

后续支持：

- Least Connections
- Consistent Hash
- Header Hash
- Cookie Affinity

### 6.1.5 Health Check

支持主动健康检查：

- HTTP GET
- TCP Dial
- Interval
- Timeout
- Healthy threshold
- Unhealthy threshold

支持被动摘除：

- 连接失败
- 5xx 比例
- 超时次数

### 6.1.6 Reverse Proxy

第一阶段基于 Go 标准库实现：

- `net/http`
- `httputil.ReverseProxy`
- `http.Transport`

支持：

- HTTP/1.1
- HTTP/2
- WebSocket
- SSE
- Streaming
- X-Forwarded-For
- X-Forwarded-Proto
- X-Request-ID
- 超时配置
- 连接池配置

### 6.1.7 Hot Reload

配置变化后不重启进程。

流程：

```text
Provider Event
  -> Build Snapshot
  -> Validate Snapshot
  -> Compile Runtime Snapshot
  -> Atomic Swap
```

请求处理路径只读取当前快照。

### 6.1.8 Access Log

访问日志独立于普通运行日志。

访问日志事件字段：

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

支持输出：

- stdout JSON
- file JSON
- batch sink plugin

### 6.1.9 Metrics

提供 Prometheus 指标：

- 请求总数
- 状态码分布
- 路由维度请求数
- 服务维度请求数
- upstream 维度请求数
- 请求耗时
- upstream 耗时
- 当前健康 endpoint 数
- 当前配置版本
- 插件状态

### 6.1.10 File Provider

第一阶段必须内置 File Provider。

支持：

- HCL 配置
- 配置文件 watch
- 配置校验
- 错误回滚

---

## 6.2 第二阶段功能

### 6.2.1 Embedded API

支持以 Go SDK 的方式嵌入其他系统：

```go
gw, err := gateway.New(gateway.Options{
    Logger:   logger,
    Provider: orchProvider,
})

if err != nil {
    return err
}

if err := gw.Start(ctx); err != nil {
    return err
}
```

嵌入模式下宿主可以控制：

- Provider 来源
- 日志实现
- Metrics 注册器
- 插件是否启用
- 配置目录
- 生命周期

### 6.2.2 Orch Provider

为 `orch` 提供内置 Provider。

Orch Provider 负责从 `orch registry` 获取：

- 服务定义
- 实例 endpoint
- 健康状态
- 路由规则
- 权重
- 灰度配置

### 6.2.3 Docker / Swarm Provider

支持通过 Docker labels / Swarm service labels 自动生成路由。

示例：

```yaml
labels:
  vale.enable: "true"
  vale.http.routers.api.rule: "Host(`api.example.com`)"
  vale.http.routers.api.service: "api"
  vale.http.services.api.loadbalancer.server.port: "8080"
```

---

## 6.3 第三阶段功能

### 6.3.1 Plugin Registry

支持从私有 Registry 安装插件。

Registry 类型：

- OCI Registry
- HTTP Registry
- File Registry
- S3-compatible Registry

建议优先实现：

1. File Registry
2. HTTP Registry
3. OCI Registry

插件安装命令示例：

```bash
vale plugin install oci://registry.example.com/vale/plugins/authz:1.0.0
```

### 6.3.2 RPC Plugin Runtime

插件以独立进程运行，通过 RPC/gRPC 与宿主通信。

支持插件类型：

- Provider Plugin
- Middleware Plugin
- Access Log Sink Plugin
- Certificate Plugin
- Metrics Exporter Plugin
- Audit Plugin

### 6.3.3 Plugin Supervisor

插件由宿主进程管理生命周期：

```text
Install
  -> Resolve
  -> Download
  -> Verify
  -> Unpack
  -> Start Process
  -> Handshake
  -> Init
  -> Ready
```

插件状态：

- Installed
- Starting
- Ready
- Degraded
- Failed
- Stopped

### 6.3.4 Plugin Upgrade

插件升级流程：

```text
Download new version
  -> Verify
  -> Start new plugin process
  -> Handshake
  -> Warm up
  -> Atomic switch
  -> Stop old plugin process
```

---

## 6.4 第四阶段功能

### 6.4.1 Cluster Mode

支持集群模式，节点角色包括：

```text
manager
proxy
manager,proxy
```

Manager 节点负责：

- Raft
- 配置存储
- Provider watch
- Snapshot build
- 插件元信息管理
- 证书状态管理

Proxy 节点负责：

- 请求转发
- 本地快照消费
- 健康检查
- 本地 access log
- metrics 上报

### 6.4.2 Raft 控制面

Raft 只用于控制面状态，不进入请求路径。

同步内容：

- 静态配置
- 动态配置版本
- 插件安装状态
- 证书元数据
- 集群成员信息

不同步请求级数据。

### 6.4.3 Snapshot Distribution

Manager 编译配置快照后分发到 Proxy 节点。

Proxy 节点本地原子切换快照。

请求路径永远只依赖本地快照。

---

## 7. 插件系统设计

## 7.1 插件设计目标

Vale 插件系统的目标是提供一个可私有化、可版本化、可校验、可隔离的扩展机制。

核心特点：

1. 插件不依赖 GitHub 官方目录。
2. 插件可从私有 Registry 安装。
3. 插件以预编译二进制分发。
4. 插件作为独立进程运行。
5. 插件通过 RPC/gRPC 与宿主通信。
6. 插件能力通过 manifest 显式声明。
7. 插件错误策略由配置显式定义。

---

## 7.2 插件类型

### 7.2.1 Provider Plugin

用于扩展服务发现或配置来源。

适用场景：

- Nacos
- Consul
- Etcd
- 自研 CMDB
- 自研注册中心
- 第三方服务目录

Provider Plugin 运行在控制面，不在请求热路径中，最适合 RPC 插件模式。

### 7.2.2 Middleware Plugin

用于请求链路中的决策和轻量修改。

适用场景：

- 认证
- 鉴权
- Header 改写
- 签名校验
- IP 黑白名单
- 请求准入
- 简单限流决策

第一阶段 Middleware Plugin 不处理 body。

### 7.2.3 Access Log Sink Plugin

用于扩展访问日志输出目的地。

适用场景：

- Kafka
- Loki
- ClickHouse
- Elasticsearch
- 自研日志系统

Access Log Sink Plugin 应使用异步 batch 写入，不影响请求路径。

### 7.2.4 Certificate Plugin

用于扩展证书存储和加载。

适用场景：

- 本地文件
- S3
- Vault
- 企业内部证书平台
- 自研证书中心

### 7.2.5 Audit Plugin

用于记录配置变更、插件安装、集群状态变化等审计事件。

---

## 7.3 插件包结构

插件包建议结构：

```text
plugin-name/
  manifest.hcl
  checksums.txt
  signatures/
  linux-amd64/plugin
  linux-arm64/plugin
  darwin-arm64/plugin
  windows-amd64/plugin.exe
```

manifest 示例：

```hcl
plugin "authz" {
  namespace   = "corp"
  name        = "authz"
  version     = "1.0.0"
  type        = "middleware"
  protocol    = "grpc"
  api_version = "vale.plugin.v1"

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

---

## 7.4 插件安全策略

插件安全策略包括：

1. Checksum 校验
2. Signature 校验
3. Registry trust policy
4. Protocol version 校验
5. Capability 校验
6. Timeout 控制
7. Fail-open / fail-closed 策略
8. 插件进程隔离
9. 插件运行目录隔离
10. 资源限制

中间件插件示例：

```hcl
middleware "corp-authz" {
  plugin  = "corp/authz"
  version = "1.0.0"
  timeout = "50ms"
  on_error = "reject"
}
```

`on_error` 可选值：

- reject
- continue
- fallback

认证、鉴权类插件默认建议 fail-closed，即 `reject`。

日志、审计类插件默认建议 fail-open，即不影响主请求。

---

## 8. 架构设计

## 8.1 总体架构

```text
                 +----------------------+
                 |      Providers       |
                 | file/docker/orch/... |
                 +----------+-----------+
                            |
                            v
                 +----------------------+
                 |  Config Aggregator   |
                 +----------+-----------+
                            |
                            v
                 +----------------------+
                 | Snapshot Validator   |
                 +----------+-----------+
                            |
                            v
                 +----------------------+
                 | Snapshot Compiler    |
                 +----------+-----------+
                            |
                            v
                 +----------------------+
                 |  Runtime Snapshot    |
                 +----------+-----------+
                            |
                            v
+--------+    +----------------------+    +-----------+
| Client | -> |  Vale Proxy Runtime  | -> | Upstream  |
+--------+    +----------------------+    +-----------+
```

---

## 8.2 模块划分

建议仓库结构：

```text
vale/
  cmd/
    vale/                  # CLI
    valed/                 # daemon

  pkg/
    gateway/               # embedded API
    config/                # config model, parser, validator
    runtime/               # proxy runtime
    router/                # rule parser, route matcher
    proxy/                 # reverse proxy
    lb/                    # load balancing
    health/                # health checks
    tls/                   # certificate management
    accesslog/             # access log event and sinks
    metrics/               # prometheus metrics
    middleware/            # builtin middleware
    provider/              # provider interfaces and builtin providers
      file/
      docker/
      swarm/
      orch/
      static/
    plugin/                # plugin manager, registry, rpc client
      registry/
      installer/
      verifier/
      supervisor/
      protocol/
    cluster/               # raft and cluster mode, later phase
    admin/                 # admin API
    internal/
```

---

## 8.3 核心数据模型

### 8.3.1 Snapshot

```go
type Snapshot struct {
    Version     uint64
    Entrypoints map[string]Entrypoint
    Routers     map[string]Router
    Services    map[string]Service
    Middlewares map[string]Middleware
    TLS         map[string]TLSConfig
}
```

### 8.3.2 Entrypoint

```go
type Entrypoint struct {
    Name    string
    Address string
    TLS     *TLSConfig
}
```

### 8.3.3 Router

```go
type Router struct {
    Name        string
    Entrypoints []string
    Rule        string
    Service     string
    Middlewares []string
    Priority    int
}
```

### 8.3.4 Service

```go
type Service struct {
    Name      string
    LBPolicy  string
    Endpoints []Endpoint
    Health    *HealthCheck
}
```

### 8.3.5 Endpoint

```go
type Endpoint struct {
    URL    string
    Weight int
    Meta   map[string]string
}
```

---

## 8.4 请求处理流程

```text
Request In
  -> Load Current Runtime Snapshot
  -> Match Entrypoint
  -> Match Router
  -> Execute Middleware Chain
  -> Select Service
  -> Choose Endpoint
  -> Reverse Proxy
  -> Record Access Log
  -> Update Metrics
```

请求热路径特征：

- 本地内存访问
- 原子快照读取
- 路由树匹配
- 无全局锁
- 无远程配置查询
- 无 Registry 查询

---

## 8.5 配置更新流程

```text
Provider emits event
  -> Aggregator merges partial snapshots
  -> Validator checks config
  -> Compiler builds router tree and runtime structures
  -> Runtime atomic swap
  -> Old snapshot retired after drain
```

配置错误时：

- 保留旧快照
- 输出错误日志
- 暴露 metrics
- admin API 返回错误状态

---

## 9. 配置设计

## 9.1 HCL 配置示例

```hcl
entrypoint "web" {
  address = ":80"
}

entrypoint "websecure" {
  address = ":443"
  tls = true
}

router "api" {
  entrypoints = ["web"]
  rule        = "Host(`api.example.com`) && PathPrefix(`/v1`)"
  service     = "api-service"
  middlewares = ["request-id", "compress"]
}

service "api-service" {
  lb = "round_robin"

  health_check {
    path     = "/health"
    interval = "10s"
    timeout  = "2s"
  }

  endpoint {
    url    = "http://10.0.0.11:8080"
    weight = 1
  }

  endpoint {
    url    = "http://10.0.0.12:8080"
    weight = 1
  }
}

middleware "request-id" {
  type = "request_id"
}

middleware "compress" {
  type = "compress"
}
```

---

## 9.2 插件配置示例

```hcl
plugin "corp-authz" {
  source  = "oci://registry.example.com/vale/plugins/authz"
  version = "1.0.0"
}

middleware "authz" {
  type    = "plugin"
  plugin  = "corp-authz"
  timeout = "50ms"
  on_error = "reject"

  config = {
    endpoint = "https://authz.internal"
    tenant   = "default"
  }
}
```

---

## 10. CLI 设计

### 10.1 基础命令

```bash
vale version
vale validate -f vale.hcl
vale run -f vale.hcl
valed --config vale.hcl
```

### 10.2 插件命令

```bash
vale plugin search authz
vale plugin install oci://registry.example.com/vale/plugins/authz:1.0.0
vale plugin list
vale plugin inspect corp/authz
vale plugin remove corp/authz
vale plugin upgrade corp/authz --version 1.1.0
```

### 10.3 集群命令

```bash
vale cluster init
vale cluster join --manager http://10.0.0.1:9900
vale cluster members
vale cluster status
```

---

## 11. Admin API

第一阶段提供本地 Admin API。

建议默认监听：

```text
127.0.0.1:9900
```

接口包括：

```text
GET /healthz
GET /readyz
GET /metrics
GET /api/v1/runtime/snapshot
GET /api/v1/runtime/routes
GET /api/v1/runtime/services
GET /api/v1/runtime/plugins
POST /api/v1/runtime/reload
```

Admin API 默认不暴露公网。

---

## 12. 可观测性设计

## 12.1 日志分类

Vale 日志分三类：

1. Runtime Log
2. Access Log
3. Audit Log

### Runtime Log

用于记录网关自身运行状态：

- 启动
- 停止
- 配置加载
- 快照切换
- 插件启动
- Provider 错误
- 健康检查状态变化

### Access Log

用于记录请求流量。

### Audit Log

用于记录管理行为：

- 配置变更
- 插件安装
- 插件升级
- 节点加入
- 节点移除
- 证书变更

---

## 12.2 Metrics 指标

核心指标：

```text
vale_requests_total
vale_request_duration_seconds
vale_upstream_request_duration_seconds
vale_upstream_errors_total
vale_routes_total
vale_services_total
vale_endpoints_total
vale_healthy_endpoints_total
vale_snapshot_version
vale_snapshot_apply_total
vale_snapshot_apply_errors_total
vale_plugin_status
vale_plugin_rpc_duration_seconds
```

---

## 13. 安全设计

### 13.1 Admin API 安全

- 默认只监听 localhost
- 支持 token
- 支持 mTLS，后续阶段
- 支持 RBAC，后续阶段

### 13.2 插件安全

- 插件包 checksum 校验
- 插件包签名校验
- 插件 Registry trust policy
- 插件 capability 声明
- 插件 RPC 超时
- 插件错误策略
- 插件进程隔离
- 插件运行目录隔离

### 13.3 TLS 安全

第一阶段：

- 静态证书
- 文件加载
- 热更新

后续阶段：

- ACME
- 证书 Registry
- 集群证书同步
- 外部证书插件

---

## 14. Roadmap

## Phase 1：单机动态网关

目标：做出稳定的单机版本。

功能：

- `valed` 二进制
- File Provider
- HCL 配置
- HTTP reverse proxy
- Host / PathPrefix 路由
- Round Robin
- Weighted Round Robin
- Health Check
- Hot Reload
- Access Log
- Prometheus Metrics
- Admin API

交付物：

- 可运行二进制
- 基础配置示例
- 单元测试
- 集成测试
- 简单 benchmark

---

## Phase 2：嵌入式与 orch 集成

目标：让 Vale 成为 `orch` 的默认入口运行时。

功能：

- Embedded API
- Orch Provider
- 服务实例动态发现
- endpoint 动态更新
- route snapshot 热切换
- 请求链路指标接入 orch

交付物：

- Go SDK
- orch 集成示例
- embedded 模式生命周期测试

---

## Phase 3：插件体系

目标：实现私有插件生态。

功能：

- Plugin Manifest
- File Registry
- HTTP Registry
- OCI Registry
- Plugin Installer
- Plugin Verifier
- Plugin Supervisor
- Provider Plugin
- Middleware Plugin
- Access Log Sink Plugin

交付物：

- Plugin SDK
- 示例 authz plugin
- 示例 access log plugin
- 插件打包工具
- 插件安装命令

---

## Phase 4：集群模式

目标：实现轻量集群网关。

功能：

- Manager / Proxy 角色
- Raft 控制面
- Snapshot 分发
- 节点注册
- 集群成员管理
- 插件状态同步
- 证书状态同步

交付物：

- 三节点 manager 示例
- 多 proxy 节点示例
- 故障切换测试
- Snapshot 一致性测试

---

## Phase 5：生产增强

目标：增强生产环境能力。

功能：

- ACME
- Canary
- Traffic Mirroring
- Rate Limit
- Circuit Breaker
- Retry
- Request Timeout Policy
- Advanced TLS
- Dashboard

---

## 15. 技术选型建议

### 15.1 语言

Go

理由：

- 网络服务生态成熟
- 部署简单
- 单二进制友好
- 并发模型适合代理和控制面
- 与 HashiCorp 风格插件体系匹配

### 15.2 HTTP Runtime

第一阶段使用标准库：

- `net/http`
- `httputil.ReverseProxy`
- `http.Transport`

暂不使用：

- fasthttp
- 自研 HTTP 协议栈

### 15.3 配置语言

建议优先支持 HCL。

原因：

- 结构清晰
- 比 YAML 更适合复杂配置
- 与 Terraform / Nomad 风格相近
- 符合平台型工具心智

后续可增加 YAML / JSON。

### 15.4 日志

建议使用 `log/slog` 作为内部日志抽象。

访问日志单独建模为事件，不直接复用普通 logger。

### 15.5 插件机制

建议基于 HashiCorp go-plugin 思路实现 RPC 插件系统。

插件协议优先使用 gRPC。

### 15.6 集群一致性

建议使用 Raft 实现控制面状态一致性。

Raft 不进入请求路径。

---

## 16. 竞争差异

### 16.1 相比 Caddy

Vale 更强调：

- 嵌入式平台集成
- Provider 驱动服务发现
- 私有插件 Registry
- RPC 插件隔离
- 集群控制面可演进

### 16.2 相比 Traefik

Vale 更强调：

- 插件不依赖 GitHub 官方目录
- 插件以预编译二进制分发
- 支持自建 Registry
- 支持 RPC 插件进程隔离
- 支持作为库嵌入自研编排系统
- 集群能力按开源可控架构设计

### 16.3 相比 Nginx

Vale 更强调：

- 动态配置
- 服务发现
- Provider 模型
- 插件体系
- 热更新快照
- 与编排系统集成

### 16.4 相比 Envoy

Vale 更强调：

- 轻量
- 简单部署
- 单二进制
- 面向中小型私有平台
- 更低学习成本
- 更适合嵌入式场景

---

## 17. 风险与边界

### 17.1 风险一：范围膨胀

网关类项目很容易从反向代理膨胀成 API Gateway、Service Mesh、认证中心、审计中心和流量治理平台。

控制方式：

- 第一阶段只做动态反代和基础路由
- 插件先做控制面和轻量 middleware
- 不处理请求 body
- 不做完整 API 管理

### 17.2 风险二：插件热路径性能

RPC 插件跨进程调用有成本。

控制方式：

- Provider Plugin 优先
- AccessLog Plugin 异步 batch
- Middleware Plugin 限定能力
- 设置超时
- 设置错误策略
- 避免每个请求多次 RPC

### 17.3 风险三：协议兼容性

反向代理涉及 WebSocket、SSE、Streaming、Header、Timeout、KeepAlive 等细节。

控制方式：

- 第一阶段基于 Go 标准库
- 避免自研 HTTP 协议栈
- 建立完整代理测试集

### 17.4 风险四：集群复杂度

集群模式容易复杂化。

控制方式：

- 单机先稳定
- Raft 只进入控制面
- Proxy 节点请求路径完全本地化
- Manager / Proxy 职责清晰

---

## 18. MVP 验收标准

第一阶段 MVP 完成标准：

1. 可以通过 HCL 配置启动 `valed`。
2. 支持至少两个 entrypoint。
3. 支持 Host + PathPrefix 路由。
4. 支持多个 upstream endpoint。
5. 支持 Round Robin 负载均衡。
6. 支持 HTTP 健康检查。
7. 支持配置热更新。
8. 支持 JSON access log。
9. 支持 Prometheus metrics。
10. 支持 Admin API 查看当前 routes/services/endpoints。
11. 配置错误时不影响旧快照。
12. 支持 WebSocket 和 SSE 基础代理场景。
13. 可以通过 Go SDK 嵌入启动一个最小网关实例。

---

## 19. 推荐第一版实现顺序

### 第一步：Runtime 内核

- Snapshot 模型
- RuntimeSnapshot 编译
- atomic swap
- route matcher
- reverse proxy

### 第二步：File Provider

- HCL parser
- config validator
- file watch
- reload

### 第三步：基础网关能力

- entrypoint
- router
- service
- endpoint
- lb
- health check

### 第四步：可观测性

- runtime log
- access log
- metrics
- admin API

### 第五步：Embedded API

- gateway.New
- Start/Stop
- Provider 注入
- Logger 注入
- Metrics 注入

### 第六步：插件系统最小闭环

- manifest
- local plugin
- process launcher
- handshake
- provider plugin demo
- middleware plugin demo

---

## 20. 总结

Vale 的核心价值不是替代所有网关，而是解决现有网关在私有化、自研编排系统、嵌入式运行、插件分发和控制面集群化上的不匹配问题。

它的产品方向可以总结为：

```text
轻量应用网关
可独立运行
可嵌入集成
Provider 驱动
Snapshot 数据面
私有 Registry 插件生态
RPC 插件隔离
控制面集群可演进
```

Vale 不应该从一开始就追求大而全，而应该先把动态路由、服务发现、热更新、健康检查、访问日志和嵌入式运行做好。

只要第一阶段的单机网关内核足够稳定，后续 Provider、插件和集群能力都可以自然向上生长。

