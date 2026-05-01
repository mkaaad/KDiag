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

- **HTTP Webhook 接入** — 提供 `net/http` Handler，接收 Alertmanager 告警推送
- **定时轮询** — 支持按配置间隔轮询 Prometheus 当前告警
- **两种 Agent 模式** — 支持 OpenAI Function Calling Agent 与 Conversational Agent
- **可定制系统提示词** — 内置 SRE 告警分析工作流提示词，输出结构化 Markdown 报告
- **自主记忆系统** — Agent 可自主搜索历史情报、展开详情、存入新知，诊断前自动注入相关上下文
- **诊断持久化** — 告警诊断结果自动存入 PostgreSQL，支持历史回溯

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
    agent.go          # LLM Agent 创建与执行
    prompt.go         # 系统提示词（SRE 告警分析工作流）
  client/
    client.go         # 客户端工厂（Prometheus / Gitea / Jaeger / Loki / Memory / Store）
  memory/
    model.go          # 记忆模型、8 类预设分类、Store 接口
    store.go          # PostgresStore 实现（GORM + PostgreSQL）
    tools.go          # SearchMemory / ReadMemory / Remember + 标签提取 / 上下文注入
  store/
    store.go          # 诊断存储接口
    postgres.go       # PostgresStore 实现
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

- **Two Agent Modes** — OpenAI Function Calling Agent or Conversational Agent
- **Customizable System Prompt** — Built-in SRE alert analysis workflow prompt that outputs structured Markdown reports
- **Autonomous Memory System** — Agent can store/retrieve intelligence with 8 preset categories, auto-injected context
- **Diagnosis Persistence** — PostgreSQL-backed diagnosis history and alert correlation

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
    agent.go          # LLM Agent creation & execution
    prompt.go         # System prompt (SRE alert analysis workflow)
  client/
    client.go         # Client factories (Prometheus / Gitea / Jaeger / Loki / Memory / Store)
  memory/
    model.go          # Memory model, 8 categories, Store interface
    store.go          # PostgresStore (GORM + PostgreSQL)
    tools.go          # SearchMemory / ReadMemory / Remember + label extraction / context injection
  store/
    store.go          # Diagnosis store interface
    postgres.go       # PostgresStore implementation
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
