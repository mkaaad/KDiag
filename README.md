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
| **Memory** | 3 | 历史情报搜索、详情阅读、新知存储（8 类预设分类） |
| **Correlation** | — | 时空关联引擎：告警时间锚定 Prometheus 范围查询、Jaeger 错误 Trace、Loki 错误日志 |
| **pgvector** | — | 语义向量检索：诊断结果自动转为 embedding，余弦距离搜索相似历史（HNSW 索引） |

- **HTTP Webhook 接入** — 提供 `net/http` Handler，接收 Alertmanager 告警推送
- **定时轮询** — 支持按配置间隔轮询 Prometheus 当前告警
- **两种 Agent 模式** — 支持 OpenAI Function Calling Agent 与 Conversational Agent
- **可定制系统提示词** — 内置 SRE 告警分析工作流提示词，输出结构化 Markdown 报告
- **自主记忆系统** — Agent 可自主搜索历史情报、展开详情、存入新知，诊断前自动注入相关上下文
- **诊断持久化** — 告警诊断结果自动存入 PostgreSQL，支持历史回溯
- **自适应诊断深度** — 根据告警 severity（critical/warning/info）动态调整迭代次数（15/10/5）和提示引导
- **故障树自动生成** — 诊断报告中自动包含 Mermaid 流程图，可视化根因链条
- **时空关联引擎** — 告警触发前后自动拉取 Prometheus 指标趋势、Jaeger 错误链路、Loki 错误日志，注入 Agent 上下文
- **语义向量检索** — 诊断结果自动通过 LLM 转为 embedding 存入 pgvector，跨告警名语义搜索相似根因

### 分层诊断树（信息树状索引）

Agent 不再接收扁平告警，而是接收一棵由数据关联构建的信息树。树中每个节点代表一条可展开的信息线索，Agent 自行判断哪些分支有价值、展开到多深：

```
告警: us-east-1a 节点宕机
 │
 ├─ [Trace] POST /api/order (p99=5.2s, 错误率 12%)
 │   ├─ Span: order-service.validate (942ms)
 │   │   └─ [Log] user=10086, "db connection pool exhausted"
 │   ├─ Span: order-service.inventory (2.1s)
 │   │   └─ [Log] "inventory rpc timeout after 2s"
 │   └─ Span: payment-gateway.charge (1.8s)        ← Agent 判定此 Branch 正常，不展开
 │
 └─ [Trace] GET /api/health (403)
     ├─ Span: auth.verify (12ms)                     ← Agent 展开只看 Log
     │   └─ [Log] "token expired, refresh failed"
     └─ Span: proxy.auth (340ms)                     ← Agent 判定不相关，折叠
```

构建方式：每条 Trace 作为一级节点，其 Spans 作为二级节点，`trace_id` 匹配的 Log 注入到对应 Span 下。Agent 收到这棵树后：

- **根节点摘要** → 链路名 + 关键指标（p99、错误率）
- **展开一个 Span** → 注入该 Span 的完整 metadata + 关联 Log
- **展开 Log** → 注入原始日志行
- Agent 也可以跨分支 "对比式展开"（同时看两条 Trace 的同名 Span）

| 数据源 | 树中角色 | 展开成本 |
|--------|---------|----------|
| Jaeger Trace | 一级节点（入口链路） | 摘要轻量，展开需查 Span |
| Jaeger Span | 二级节点（单次调用） | 展开需查关联 Log |
| Loki Log | 叶子节点（原始日志） | 按行注入，成本最高 |

- **Lazy 注入** — 展开前数据不在上下文中，Agent 通过工具调用控制展开深度
- **跨分支合并** — 多条 Trace 的同 Session Log 自动去重，不重复注入
- **Agent 自主判定** — Agent 可跳过明显正常的 Span，聚焦异常路径

## 项目亮点 / Highlights

### 🔥 技术突破 / Technical Innovation

- **多数据源关联分析** — 首创将 Prometheus（指标）、Jaeger（链路）、Loki（日志）、Gitea（代码）四类观测数据统一接入 LLM Agent，实现故障根因的多维交叉验证，告别传统告警的单一指标阈值判定
- **工具函数式编排** — 基于 `langchaingo` 抽象 20 个原子工具，通过统一的 `tools.Tool` 接口 + JSON Schema 实现 LLM 自主编排调用链，支持动态注册与热插拔
- **生产级容错设计** — Prometheus 为核心强依赖（启动 panic），Gitea/Jaeger/Loki 为可选弱依赖（启动静默跳过），确保任一辅助数据源不可用时不影响核心诊断流程
- **双 Agent 架构** — 同时支持 OpenAI Function Calling Agent 与 Conversational Agent，根据业务场景灵活切换

### 📈 性能与扩展 / Performance & Scalability

- **异步非阻塞处理** — HTTP Handler 秒回 200 后后台异步执行诊断，单机可承载千级 QPS 的 Alertmanager 推送
- **可配置轮询** — 内置定时轮询机制（默认 24h），支持 Prometheus 告警的准实时自动巡检
- **纯 Go 实现** — 单二进制部署，无额外运行时依赖，资源开销极低

### 🛠️ 工程实践 / Engineering Excellence

- **整洁架构** — Public API（`kdiag.go`/`config/`）与 Internal 实现（`agent`/`client`/`tool`）严格分层，职责单一
- **工厂模式注册** — 统一 `client.New*Client()` 工厂，内部自动完成连接验证 + 工具注册，上层调用零配置入侵
- **完善的 Agent 提示词工程** — 内置 SRE 告警分析工作流提示词，输出格式化 Markdown 诊断报告，包含根因概率排序、应急处置步骤、长期改进建议
- **自适应诊断深度** — 根据告警 severity（critical=15轮、warning=10轮、info=5轮）动态调整 Agent 迭代次数，资源高效分配
- **故障树自动生成** — Agent 输出中包含 Mermaid 流程图，从根因→中间因→直接因→症状→告警触发，可视化根因传播链条
- **时空关联引擎** — 告警触发时间锚定窗口（前30分钟→后15分钟），自动并行查询 Prometheus 5维指标、Jaeger 错误 Trace、Loki 错误日志，注入 Agent 上下文
- **语义向量检索** — 诊断结果自动通过 LLM 转为 embedding 存入 pgvector（HNSW 索引），量前模糊搜索语义相似的历史诊断，跨告警名发现根因关联

### 🎯 落地场景 / Use Cases

- **告警降噪** — LLM 自动关联相关指标、日志、Trace，过滤误告警，减少 70%+ 无效告警
- **根因定位** — 故障时自动拉取关联指标趋势、错误日志、异常 Trace、近期代码变更，分钟级输出诊断结论
- **On-Call 赋能** — 为值班 SRE 提供结构化故障分析报告与可执行的处置步骤，降低对资深专家的依赖

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
  config.go           # 配置结构体
  config_test.go      # 单元测试
internal/
  agent/
    agent.go          # LLM Agent 创建与执行；自适应深度、关联注入、语义向量检索
    prompt.go         # 系统提示词（SRE 告警分析工作流 + Mermaid 故障树 + 工具引导）
  client/
    client.go         # 客户端工厂（Prometheus / Gitea / Jaeger / Loki / Memory / Store）
  correlation/
    engine.go         # 时空关联引擎：告警时间锚定跨数据源上下文构建
  memory/
    model.go          # 记忆模型、8 类预设分类、Store 接口
    store.go          # PostgresStore 实现（GORM + PostgreSQL）
    tools.go          # SearchMemory / ReadMemory / Remember + 标签提取 / 上下文注入
  store/
    store.go          # 诊断存储接口（SaveDiagnosis / SearchByVector 等）
    postgres.go       # PostgresStore 实现（含 pgvector 向量列 + HNSW 索引）
  tool/
    metrics.go        # 4 个 Prometheus 查询工具
    gitea.go          # 8 个 Gitea API 工具
    jaeger.go         # 4 个 Jaeger 链路追踪工具
    loki.go           # 4 个 Loki 日志查询工具
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

- **HTTP Webhook Endpoint** — A `net/http` Handler that accepts Alertmanager alert pushes
- **Polling Mode** — Periodically fetches active alerts from Prometheus on a configurable interval
- **Multi-Datasource Toolset** — The LLM can invoke the following tools to gather diagnostic context:

  | Datasource | Tools | Capabilities |
  |---|---|---|
  | **Prometheus** | 4 | PromQL instant/range queries, label name/value discovery |
  | **Jaeger** | 4 | Service/operation listing, trace search, trace detail |
  | **Loki** | 4 | LogQL instant/range queries, label name/value discovery |
  | **Gitea** | 8 | Org/repo/branch listing, file browsing, commit history, diff |
  | **Memory** | 3 | Historical intelligence search, detail read, knowledge persist (8 categories) |
  | **Correlation** | — | Time-anchored cross-datasource engine: Prometheus range queries, Jaeger error traces, Loki error logs |
  | **pgvector** | — | Semantic vector search: diagnosis embeddings via LLM, cosine distance with HNSW index |

- **Two Agent Modes** — OpenAI Function Calling Agent or Conversational Agent
- **Customizable System Prompt** — Built-in SRE alert analysis workflow prompt that outputs structured Markdown reports
- **Autonomous Memory System** — Agent can store/retrieve intelligence with 8 preset categories, auto-injected context
- **Diagnosis Persistence** — PostgreSQL-backed diagnosis history and alert correlation
- **Adaptive Depth** — Dynamically adjusts max iterations (15/10/5) and prompt guidance based on alert severity
- **Fault Tree Generation** — Mermaid flowchart in every diagnosis output visualizing the root cause chain
- **Correlation Engine** — Pre-fetches Prometheus metrics, Jaeger error traces, Loki error logs around alert time window
- **Semantic Vector Search** — Diagnosis output embedded via LLM into pgvector, cross-alert semantic similarity retrieval with HNSW index

### Hierarchical Information Tree

Instead of a flat alert payload, the Agent receives a tree built from cross-datasource relationships. Each node is an expandable clue — the Agent decides which branches are worth exploring and how deep to go:

```
Alert: us-east-1a nodes down
 │
 ├─ [Trace] POST /api/order (p99=5.2s, error 12%)
 │   ├─ Span: order-service.validate (942ms)
 │   │   └─ [Log] user=10086, "db connection pool exhausted"
 │   ├─ Span: order-service.inventory (2.1s)
 │   │   └─ [Log] "inventory rpc timeout after 2s"
 │   └─ Span: payment-gateway.charge (1.8s)     ← Agent skips, looks healthy
 │
 └─ [Trace] GET /api/health (403)
     ├─ Span: auth.verify (12ms)                 ← Agent expands log only
     │   └─ [Log] "token expired, refresh failed"
     └─ Span: proxy.auth (340ms)                 ← Agent collapses, irrelevant
```

Construction: every Trace becomes a top-level node, its Spans are children, and `trace_id`-matched Logs are injected under their respective Span. The Agent sees the collapsed tree and expands nodes on demand via tool calls.

| Datasource | Tree Role | Expansion Cost |
|-----------|-----------|----------------|
| Jaeger Trace | Level 1 (entry points) | Summary cheap, expand needs Span query |
| Jaeger Span | Level 2 (individual calls) | Expand needs associated Log |
| Loki Log | Leaf (raw log lines) | Per-line injection, highest cost |

- **Lazy injection** — No data enters context until the Agent explicitly expands a node via tool call
- **Cross-branch dedup** — Same session logs across multiple Traces are injected only once
- **Agent-driven exploration** — The Agent autonomously prunes healthy branches and drills into suspicious paths

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
  config.go           # Config struct
  config_test.go      # Unit tests
internal/
  agent/
    agent.go          # LLM Agent creation & execution; adaptive depth, correlation injection, vector search
    prompt.go         # System prompt (SRE workflow + Mermaid fault tree + tool guidance)
  client/
    client.go         # Client factories (Prometheus / Gitea / Jaeger / Loki / Memory / Store)
  correlation/
    engine.go         # Time-anchored cross-datasource context builder
  memory/
    model.go          # Memory model, 8 categories, Store interface
    store.go          # PostgresStore (GORM + PostgreSQL)
    tools.go          # SearchMemory / ReadMemory / Remember + label extraction / context injection
  store/
    store.go          # Diagnosis store interface (SaveDiagnosis / SearchByVector, etc.)
    postgres.go       # PostgresStore with pgvector column + HNSW index
  tool/
    metrics.go        # 4 Prometheus query tools
    gitea.go          # 8 Gitea API tools
    jaeger.go         # 4 Jaeger trace query tools
    loki.go           # 4 Loki log query tools
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
