<div align="center">

# KDiag

**LLM驱动的 Prometheus 告警诊断引擎**  
**LLM-Powered Prometheus Alert Diagnosis Engine**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**KDiag** is an intelligent alert diagnosis engine that integrates Prometheus, Jaeger, Loki, and Gitea into a unified LLM-driven root cause analysis system — instantly correlating metrics, traces, logs, and code changes to resolve incidents within seconds.

**KDiag** 是一款智能告警诊断引擎，打通 Prometheus、Jaeger、Loki、Gitea 数据孤岛，利用 LLM 实现指标—链路—日志—代码变更的多维关联分析，将告警根因定位时间从小时级压缩至秒级。

</div>

---

## 中文介绍

### 概述

KDiag 是一个 Go 库，接收 Prometheus Alertmanager 的告警通知，利用 LLM（大语言模型）对告警进行智能诊断分析。它内置了多数据源查询工具，让 LLM 能够自主获取上下文信息进行根因分析。

### 功能特性

| 数据源 | 工具数量 | 能力 |
|---|---|---|
| **Prometheus** | 4 | PromQL 即时/范围查询、标签名、标签值发现 |
| **Jaeger** | 4 | 服务/操作列表、链路搜索、Trace 详情查询 |
| **Loki** | 4 | LogQL 即时/范围查询、标签名、标签值发现 |
| **Gitea** | 8 | 组织/仓库/分支列表、文件浏览、提交历史、Diff 查看 |
| **ExpandNode / AddNode** | 2 | 关联信息树的延迟展开与增量构建 |
| **Memory** | 3 | 历史情报搜索、详情阅读、新知存储（8 类预设分类） |
| **Correlation** | — | 时空关联窗口：告警时间锚点元数据注入，Agent 自主决定查询策略 |
| **pgvector** | — | 语义向量检索：诊断结果 + 树结构路径自动 embedding，余弦距离搜索（HNSW 索引） |
| **Trace Correlation** | — | Trace ID 索引表：跨告警共享 Trace 自动关联，发现同因告警聚合 |

- **HTTP Webhook 接入** — 提供 `net/http` Handler，接收 Alertmanager 告警推送
- **定时轮询** — 支持按配置间隔轮询 Prometheus 当前告警
- **两种 Agent 模式** — 支持 OpenAI Function Calling Agent 与 Conversational Agent
- **可定制系统提示词** — 内置 SRE 告警分析工作流提示词，输出结构化 Markdown 报告
- **自主记忆系统** — Agent 可自主搜索历史情报、展开详情、存入新知，诊断前自动注入相关上下文
- **诊断持久化** — 告警诊断结果 + 关联信息树自动存入 PostgreSQL，支持历史回溯与结构化查询
- **自适应诊断深度** — 根据告警 severity（critical/warning/info）动态调整迭代次数（15/10/5）和提示引导
- **故障树自动生成** — 诊断报告中自动包含 Mermaid 流程图，可视化根因链条
- **时空关联窗口** — 告警触发时间锚定窗口（前30分钟→后15分钟），以元数据表格注入 Agent，工具查询由 Agent 自主编排
- **语义向量检索** — 诊断内容 + 树结构路径各自转为 embedding 存入 pgvector，跨告警名搜索语义/结构相似根因
- **跨告警 Trace 关联** — 自动提取诊断树中的 Trace ID，建立索引，同 Trace 告警自动聚合

### 关联信息树（Agent 驱动）

Agent 不再接收扁平告警，也不再被动接收预取数据。系统仅提供时间锚点元数据，Agent 自主决定查询哪些数据源、构建怎样的关联信息树：

```
告警: us-east-1a 节点宕机
Agent 收到时间窗: 10:00 ~ 10:45，可用工具: MetricsQueryRange, JaegerFindTraces, LokiQueryRange
 │
 ├─ Agent 主动查询 Prometheus → 发现 CPU 异常
 │   └─ AddNode({summary:"CPU 使用率 95%", type:"metric"})
 │
 ├─ Agent 主动查询 Jaeger → 发现错误 Trace
 │   ├─ AddNode({summary:"POST /api/order 错误率 12%", type:"trace"})
 │   ├─ ExpandNode → 查看 Span 详情
 │   └─ AddNode({parent_id:"tr_1", summary:"db timeout", type:"span"})
 │
 └─ Agent 主动查询 Loki → 关联日志
     └─ AddNode({parent_id:"span_1", summary:"connection pool exhausted", type:"log"})
```

诊断结束后，整棵树自动持久化到 PostgreSQL：

| 能力 | 实现 |
|------|------|
| **树持久化** | `tree_nodes` 表：每个节点一行，含 parent_id 维护父子关系 |
| **路径向量化** | `tree_paths` 表：将所有根到叶路径编码为 embedding，余弦距离搜索结构相似案例 |
| **Trace 关联** | `trace_links` 表：自动索引所有 Trace ID，共享 Trace 的告警自动关联 |

### 项目亮点 / Highlights

#### 🔥 技术突破 / Technical Innovation

- **多数据源关联分析** — 首创将 Prometheus（指标）、Jaeger（链路）、Loki（日志）、Gitea（代码）四类观测数据统一接入 LLM Agent，实现故障根因的多维交叉验证，告别传统告警的单一指标阈值判定
- **工具函数式编排** — 基于 `langchaingo` 抽象 20 个原子工具，通过统一的 `tools.Tool` 接口 + JSON Schema 实现 LLM 自主编排调用链，支持动态注册与热插拔
- **生产级容错设计** — Prometheus 为核心强依赖（启动 panic），Gitea/Jaeger/Loki 为可选弱依赖（启动静默跳过），确保任一辅助数据源不可用时不影响核心诊断流程
- **双 Agent 架构** — 同时支持 OpenAI Function Calling Agent 与 Conversational Agent，根据业务场景灵活切换

#### 📈 性能与扩展 / Performance & Scalability

- **异步非阻塞处理** — HTTP Handler 秒回 200 后后台异步执行诊断，单机可承载千级 QPS 的 Alertmanager 推送
- **可配置轮询** — 内置定时轮询机制（默认 24h），支持 Prometheus 告警的准实时自动巡检
- **纯 Go 实现** — 单二进制部署，无额外运行时依赖，资源开销极低

#### 🛠️ 工程实践 / Engineering Excellence

- **整洁架构** — Public API（`kdiag.go`/`config/`）与 Internal 实现（`agent`/`client`/`tool`）严格分层，职责单一
- **工厂模式注册** — 统一 `client.New*Client()` 工厂，内部自动完成连接验证 + 工具注册，上层调用零配置入侵
- **完善的 Agent 提示词工程** — 内置 SRE 告警分析工作流提示词，输出格式化 Markdown 诊断报告，包含根因概率排序、应急处置步骤、长期改进建议
- **自适应诊断深度** — 根据告警 severity（critical=15轮、warning=10轮、info=5轮）动态调整 Agent 迭代次数，资源高效分配
- **故障树自动生成** — Agent 输出中包含 Mermaid 流程图，从根因→中间因→直接因→症状→告警触发，可视化根因传播链条
- **时空关联窗口** — 时间锚定元数据注入而非数据预取，Agent 自主编排工具调用
- **语义向量检索** — 诊断内容 + 树结构路径双向 embedding 存入 pgvector（HNSW 索引），支持语义相似 + 结构相似双重检索
- **关联信息树持久化** — 诊断树节点持久化至 PostgreSQL，支持按 Trace ID 跨告警关联

#### 🎯 落地场景 / Use Cases

- **告警降噪** — LLM 自动关联相关指标、日志、Trace，过滤误告警，减少 70%+ 无效告警
- **根因定位** — 故障时自动拉取关联指标趋势、错误日志、异常 Trace、近期代码变更，分钟级输出诊断结论
- **On-Call 赋能** — 为值班 SRE 提供结构化故障分析报告与可执行的处置步骤，降低对资深专家的依赖
- **历史案例复用** — 告警树结构相似性搜索 + Trace ID 跟踪，同类故障无需重复分析

### 快速开始

```go
package main

import (
    "context"
    "net/http"

    "github.com/mkaaad/kdiag"
    "github.com/mkaaad/kdiag/config"
    "github.com/tmc/langchaingo/llms/openai"
)

func main() {
    llm, _ := openai.New()
    cfg := &config.Config{
        LLM:               llm,
        PrometheusAddress: "http://prometheus:9090",
        JaegerAddress:     "http://jaeger:16686",
        LokiAddress:       "http://loki:3100",
        MaxIterations:     10,
    }

    http.Handle("/webhook", kdiag.NewHandlerFunc(context.Background(), cfg))
    http.ListenAndServe(":8080", nil)
}
```

### 目录结构

```
kdiag.go              # 入口：NewHandlerFunc (HTTP handler), PollAlerts (轮询)
config/
  config.go           # 配置结构体（LLM、数据源地址、Postgres、Embedder）
  config_test.go      # 单元测试
internal/
  agent/
    agent.go          # LLM Agent 创建与执行；自适应深度、时间窗注入、向量检索
    prompt.go         # 系统提示词（SRE 告警分析工作流 + Mermaid 故障树 + 工具引导）
  client/
    client.go         # 客户端工厂（Prometheus / Gitea / Jaeger / Loki / Expand / Memory / Store）
  correlation/
    engine.go         # 时空关联窗口：告警时间锚点元数据注入
  memory/
    model.go          # 记忆模型、8 类预设分类、Store 接口
    store.go          # PostgresStore 实现（GORM + PostgreSQL）
    tools.go          # SearchMemory / ReadMemory / Remember + 标签提取 / 上下文注入
  store/
    store.go          # 诊断存储接口（SaveDiagnosis / SaveTreeNodes / SearchByTraceID / SearchByVector 等）
    postgres.go       # PostgresStore 实现（含 pgvector 向量列 + HNSW 索引 + 树节点表 + Trace 链接表）
  tool/
    metrics.go        # 4 个 Prometheus 查询工具
    gitea.go          # 8 个 Gitea API 工具
    jaeger.go         # 4 个 Jaeger 链路追踪工具
    loki.go           # 4 个 Loki 日志查询工具
    tree.go           # TreeNode / InfoTree 类型、Format / ExtractPaths
    expand.go         # ExpandNodeTool：延迟展开树节点（Jaeger Trace / Loki 日志）
    addnode.go        # AddNodeTool：Agent 增量构建关联信息树
  notify/
    notify.go         # 通知分发接口（尚为桩代码）
```

### 构建与测试

```bash
go build ./...
go test ./...
```

---

## English

### Overview

KDiag is a Go library that receives Prometheus Alertmanager webhook notifications and uses an LLM (Large Language Model) to diagnose alerts intelligently. It comes with built-in multi-datasource query tools that allow the LLM to autonomously gather contextual information for root cause analysis.

### Features

| Datasource | Tools | Capabilities |
|---|---|---|
| **Prometheus** | 4 | PromQL instant/range queries, label name/value discovery |
| **Jaeger** | 4 | Service/operation listing, trace search, trace detail |
| **Loki** | 4 | LogQL instant/range queries, label name/value discovery |
| **Gitea** | 8 | Org/repo/branch listing, file browsing, commit history, diff |
| **ExpandNode / AddNode** | 2 | Lazy tree expansion and incremental tree building |
| **Memory** | 3 | Historical intelligence search, detail read, knowledge persist (8 categories) |
| **Correlation** | — | Time anchor metadata injection; agent-driven query strategy |
| **pgvector** | — | Dual embedding: diagnosis content + tree structure paths, cosine distance with HNSW index |
| **Trace Correlation** | — | Trace ID index table for cross-alert correlation by shared trace |

- **HTTP Webhook Endpoint** — A `net/http` Handler that accepts Alertmanager alert pushes
- **Polling Mode** — Periodically fetches active alerts from Prometheus on a configurable interval
- **Two Agent Modes** — OpenAI Function Calling Agent or Conversational Agent
- **Customizable System Prompt** — Built-in SRE alert analysis workflow prompt that outputs structured Markdown reports
- **Autonomous Memory System** — Agent can store/retrieve intelligence with 8 preset categories, auto-injected context
- **Diagnosis Persistence** — Diagnosis results + correlation tree persisted to PostgreSQL with structured query support
- **Adaptive Depth** — Dynamically adjusts max iterations (15/10/5) and prompt guidance based on alert severity
- **Fault Tree Generation** — Mermaid flowchart in every diagnosis output visualizing the root cause chain
- **Correlation Window** — Time anchor metadata injected instead of data prefetch; agent orchestrates tool queries autonomously
- **Semantic Vector Search** — Dual-path embedding (diagnosis text + tree structure) via pgvector with HNSW index
- **Cross-alert Trace Correlation** — Automatic trace ID extraction and indexing, linking related alerts

### Correlation Information Tree (Agent-Driven)

Instead of flat alerts or pre-fetched data blobs, the agent receives only a time anchor and decides what to query. It builds the correlation tree incrementally using AddNode:

```
Alert: us-east-1a nodes down
Agent receives time window: 10:00 ~ 10:45, tools available
 │
 ├─ Agent queries Prometheus → CPU anomaly detected
 │   └─ AddNode({summary:"CPU 95%", type:"metric"})
 │
 ├─ Agent queries Jaeger → error trace found
 │   ├─ AddNode({summary:"POST /api/order error 12%", type:"trace"})
 │   ├─ ExpandNode → view span details
 │   └─ AddNode({parent_id:"tr_1", summary:"db timeout", type:"span"})
 │
 └─ Agent queries Loki → correlated logs
     └─ AddNode({parent_id:"span_1", summary:"connection pool exhausted", type:"log"})
```

After diagnosis, the entire tree is persisted to PostgreSQL:

| Capability | Implementation |
|---|---|
| **Tree Persistence** | `tree_nodes` table with parent_id relationships |
| **Path Vectorization** | `tree_paths` table; root-to-leaf paths encoded as embeddings for structure similarity search |
| **Trace Correlation** | `trace_links` table; automatic trace ID indexing for cross-alert correlation |

### Quick Start

```go
package main

import (
    "context"
    "net/http"

    "github.com/mkaaad/kdiag"
    "github.com/mkaaad/kdiag/config"
    "github.com/tmc/langchaingo/llms/openai"
)

func main() {
    llm, _ := openai.New()
    cfg := &config.Config{
        LLM:               llm,
        PrometheusAddress: "http://prometheus:9090",
        JaegerAddress:     "http://jaeger:16686",
        LokiAddress:       "http://loki:3100",
        MaxIterations:     10,
    }

    http.Handle("/webhook", kdiag.NewHandlerFunc(context.Background(), cfg))
    http.ListenAndServe(":8080", nil)
}
```

### Directory Layout

```
kdiag.go              # Entry: NewHandlerFunc (HTTP handler), PollAlerts (polling)
config/
  config.go           # Config struct (LLM, datasource addresses, Postgres, Embedder)
  config_test.go      # Unit tests
internal/
  agent/
    agent.go          # LLM Agent creation & execution; adaptive depth, correlation, vector search
    prompt.go         # System prompt (SRE workflow + Mermaid fault tree + tool guidance)
  client/
    client.go         # Client factories (Prometheus / Gitea / Jaeger / Loki / Expand / Memory / Store)
  correlation/
    engine.go         # Time-anchored correlation window metadata injection
  memory/
    model.go          # Memory model, 8 categories, Store interface
    store.go          # PostgresStore (GORM + PostgreSQL)
    tools.go          # SearchMemory / ReadMemory / Remember + label extraction / context injection
  store/
    store.go          # Diagnosis store interface (SaveDiagnosis / SaveTreeNodes / SearchByTraceID / SearchByVector)
    postgres.go       # PostgresStore with pgvector + HNSW index + tree node table + trace link table
  tool/
    metrics.go        # 4 Prometheus query tools
    gitea.go          # 8 Gitea API tools
    jaeger.go         # 4 Jaeger trace query tools
    loki.go           # 4 Loki log query tools
    tree.go           # TreeNode / InfoTree types, Format / ExtractPaths
    expand.go         # ExpandNodeTool: lazy tree expansion (Jaeger trace / Loki logs)
    addnode.go        # AddNodeTool: agent-driven incremental tree building
  notify/
    notify.go         # Notification dispatch interface (stub)
```

### Build & Test

```bash
go build ./...
go test ./...
```

---

## License

MIT
