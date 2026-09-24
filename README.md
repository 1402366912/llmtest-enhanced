# llmtest-enhanced: 大语言模型 API 性能测试工具

[![Go版本](https://img.shields.io/badge/Go-1.18+-blue.svg)](https://golang.org/doc/devel/release.html)
[![许可证](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)

llmtest-enhanced 提供多模型 API 性能测试、并发压测、隔离的 Prefill/Decode/MTP 基准、调度竞争测试、性能报告和 Web 界面。

本仓库保留所使用的 MIT 授权代码及原版权声明；新增的隔离基准与集成功能由本仓库维护。

## 目录

- [功能特点](#功能特点)
- [快速开始](#快速开始)
- [安装](#安装)
- [使用方式](#使用方式)
- [配置文件](#配置文件)
- [输出报告](#输出报告)
- [命令行选项](#命令行选项)
- [Web 界面](#web-界面)
- [贡献](#贡献)
- [许可证](#许可证)

## 功能特点

### 核心功能
- ✅ 支持多种 LLM 模型（OpenAI、Anthropic、Gemini 等）
- ✅ 可配置的并发度测试，支持模型特定的并发度设置
- ✅ 详细的性能指标（延迟、吞吐量、成功率、Token 处理速度等）
- ✅ 延迟百分位数统计（P50、P90、P95、P99）
- ✅ 支持多种输出格式（文本、CSV、JSON、Excel）
- ✅ 代理支持，可为不同模型配置不同代理
- ✅ 流式输出支持
- ✅ 可配置的测试参数（持续时间、预热时间、超时等）
- ✅ 数据集批量测试支持
- ✅ Prefill 性能指标（首 Token 延迟）

### Web 界面特性
- 🌐 可视化配置管理和实时测试监控
- 🖥️ 多机器管理（支持多环境隔离）
- 📊 实时性能图表和天梯图对比
- 📝 测试历史记录和结果导出
- 🔄 WebSocket 实时进度推送
- 📈 ECharts 可视化性能分析

## 快速开始

### 方式一：Web 界面（推荐）

适合大多数用户，提供可视化操作界面。

**Windows:**
```bash
start.bat
```

**Linux / macOS:**
```bash
chmod +x build.sh
./build.sh
./llm-test-server
```

启动后访问：**http://localhost:8080**

### 方式二：命令行

适合自动化测试和 CI/CD 集成。

```bash
# 构建
go build -o llm-test

# 运行测试
./llm-test -c config.yaml

# 指定输出格式
./llm-test -c config.yaml -f json -o result.json
```

## 安装

### 前提条件

- **Go 1.18+** - 后端开发和命令行工具
- **Node.js 18+** - 仅 Web 界面需要
- **Git** - 克隆代码仓库

### 从源码安装

```bash
# 克隆仓库
git clone https://github.com/1402366912/llmtest-enhanced.git
cd llmtest-enhanced

# 方式一：构建 Web 界面版本（推荐）
./build.sh      # Linux/macOS
build.bat       # Windows

# 方式二：仅构建命令行版本
go build -o llm-test main.go
```

### 使用 Make（Linux/macOS）

```bash
make build      # 构建后端
make run        # 运行服务器
make clean      # 清理构建产物
make help       # 查看帮助
```

## 使用方式

### HTML 等价的隔离 Prefill/Decode 基准

主测试引擎用于持续压测，其 `Prefill TPS` 包含请求排队和并发争用，不能与单请求
内核速度直接比较。需要复现 `本地大模型推理速度测试工具v2.2.html` 的单请求口径时，
使用 `tools/prefill_bench.py`：

```bash
python3 -m pip install requests
python3 tools/prefill_bench.py \
  --url http://127.0.0.1:60015 \
  --model Qwen3.6-27B-AWQ \
  --lengths 10000,50000,90000,130000 \
  --output-length 128 \
  --warmup-runs 1 \
  --runs 1 \
  --output-json isolated-prefill.json
```

也可以通过 Makefile 调用：

```bash
make prefill-bench PREFILL_ARGS="--url http://127.0.0.1:60015 --lengths 10000,50000"
```

计量规则：

- 整组只在第一个长度执行一次 warmup，不会每个档位重复预热。
- TTFT 从发起请求计到第一个非空 reasoning/content token。
- Prefill 使用 API `usage.prompt_tokens / TTFT`，不使用本地估算 token 数。
- Decode 使用 API 输出 token 数和首 token 后的流式耗时。
- 每次启动自动在提示词最前面加入唯一 nonce，避免 vLLM Prefix Cache 把 Prefill 虚高。
- 默认每档只跑一次；`--runs` 大于 1 时才报告中位数。

该工具报告的是隔离单请求性能；主引擎报告的是负载下性能，两者应分别保留，
不要放在同一排行榜列中比较。

### Web 界面使用

详细的 Web 界面使用教程请参阅 [README_WEB.md](README_WEB.md)

**快速上手：**
1. 启动服务器：`start.bat`（Windows）或 `./llm-test-server`（Linux/macOS）
2. 访问 http://localhost:8080
3. 创建机器并配置后端
4. 设置测试参数和提示词
5. 点击"开始测试"并查看实时结果

### 命令行使用

```bash
# 基本用法
./llm-test -c config.yaml

# 指定输出格式和文件
./llm-test -c config.yaml -f csv -o report.csv
./llm-test -c config.yaml -f json -o report.json

# 查看帮助
./llm-test -h
```

## 配置文件

配置文件 `config.yaml` 包含所有测试配置，包括测试参数、模型配置和提示词设置。

### 完整配置示例

```yaml
# 测试配置
test:
  # 基础并发数
  concurrency: 10
  # 测试持续时间
  duration: 30s
  # 预热时间
  warmup_duration: 10s
  # 请求超时
  request_timeout: 10m
  # 并发级别列表（会依次测试）
  concurrency_levels: [5, 10, 20, 50]
  # 显示进度条
  show_progress: true
  # 失败重试次数
  max_retries: 3
  # 延迟百分位数
  latency_percentiles: [50, 90, 95, 99]
  # 上下文 Token 测试级别
  context_token_levels: [1024, 2048, 4096, 8192, 16384]
  # 启用 Prefill 指标
  enable_prefill_metrics: true
  # 压力测试模式
  stress_test_mode: false

# 模型配置
models:
  - name: gpt-4
    type: openai
    api_key: sk-xxx
    base_url: https://api.openai.com/v1
    params:
      model: gpt-4
      temperature: 0.7
      max_tokens: 2048
      top_p: 1
    # 模型特定并发度（可选，覆盖全局配置）
    concurrency_levels: [5, 10, 20]
    # 使用代理（可选）
    proxy_name: "my-proxy"
    # 跳过此模型
    skip: false

  - name: claude-3
    type: anthropic
    api_key: sk-ant-xxx
    base_url: https://api.anthropic.com
    params:
      model: claude-3-opus-20240229
      temperature: 0.7
      max_tokens: 2048
    skip: false

# 代理配置
proxies:
  - name: "my-proxy"
    url: "http://proxy.example.com:8080"

# 提示词配置
prompt:
  # 系统消息
  system_message: "你是一个助手，请提供简洁明了的回答。"
  # 用户消息
  user_message: "请解释量子计算的基本原理。"
  # 数据集路径（可选，用于批量测试）
  dataset_path: "dataset/prompts.txt"
  # 启用流式输出
  stream: true
```

### 配置说明

**测试参数：**
- `concurrency`: 基础并发数
- `duration`: 每个并发级别的测试时长
- `warmup_duration`: 预热时间，让连接池预热
- `concurrency_levels`: 多级并发测试列表
- `enable_prefill_metrics`: 启用首 Token 延迟统计

**模型配置：**
- `type`: 模型类型（openai/anthropic/gemini）
- `skip`: 设为 true 可跳过该模型
- 每个模型可配置独立的并发度和代理

## 输出报告

测试完成后，工具会生成详细的性能报告，包括：

- 每个模型在不同并发度下的性能数据
- 平均延迟和延迟百分位数据（P50、P90、P95、P99）
- 请求成功率和失败率
- 每秒请求数（RPS）和每秒 Token 数（TPS）
- Token 使用统计（输入/输出/总计）
- Prefill 性能指标（首 Token 延迟）

### 示例报告

```
# LLM API 性能测试报告摘要

| 模型 | 并发度 | 成功/总请求 | 成功率 | 平均延迟 | P50 | P90 | P95 | P99 | RPS | TPS |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gpt-4 | 10 | 31/31 | 100.00% | 43.29s | 42.1s | 45.2s | 46.8s | 48.5s | 0.18 | 326.33 |
| gpt-4 | 20 | 62/62 | 100.00% | 44.80s | 43.5s | 47.1s | 48.9s | 50.2s | 0.38 | 723.54 |
| claude-3 | 10 | 30/32 | 93.75% | 38.96s | 37.8s | 41.2s | 42.5s | 44.1s | 0.21 | 351.37 |
| claude-3 | 20 | 54/62 | 87.10% | 41.30s | 40.1s | 43.8s | 45.2s | 46.9s | 0.35 | 564.73 |
```

### 输出格式

支持多种输出格式：

- **text**: 人类可读的文本格式（默认）
- **csv**: CSV 格式，便于导入 Excel
- **json**: JSON 格式，便于程序处理
- **xlsx**: Excel 格式（仅 Web 界面支持）

## 命令行选项

### CLI 工具选项

```bash
用法: llm-test [选项]

选项:
  -c, --config string   配置文件路径 (默认 "config.yaml")
  -f, --format string   报告格式: text, csv, json (默认 "text")
  -o, --output string   输出文件路径 (默认输出到控制台)
  -h, --help            显示帮助信息
```

### Web 服务器选项

```bash
用法: llm-test-server [选项]

选项:
  -c, --config string   配置文件路径 (默认 "config.yaml")
  -addr string          服务器监听地址 (默认 ":8088")
  -p string             端口简写 (如 "8080")
  -h, --help            显示帮助信息
```

**示例：**
```bash
# 使用默认端口 8088
./llm-test-server

# 指定端口
./llm-test-server -addr :9000
./llm-test-server -p 9000

# 指定配置文件
./llm-test-server -c custom-config.yaml
```

## Web 界面

Web 界面提供了更强大的功能和更好的用户体验。详细使用教程请参阅 [README_WEB.md](README_WEB.md)。

### 主要功能

- **多机器管理**: 创建和管理多个独立的测试环境
- **可视化配置**: 通过 UI 配置测试参数和模型
- **实时监控**: WebSocket 实时推送测试进度和性能指标
- **结果分析**: 性能天梯图、延迟分布图等可视化分析
- **历史记录**: 保存和对比历史测试结果
- **数据导出**: 导出 Excel、JSON 格式报告

### 快速访问

启动服务器后，访问以下地址：

- **主页**: http://localhost:8080
- **API 文档**: http://localhost:8080/api/
- **WebSocket**: ws://localhost:8080/api/ws

## 项目结构

```
llm-test/
├── cmd/
│   └── server/           # Web 服务器入口
├── api/                  # API 服务层
│   ├── server.go         # 服务器核心
│   ├── test.go           # 测试控制
│   ├── websocket.go      # WebSocket 实时通信
│   ├── machines.go       # 机器管理
│   ├── backends.go       # 后端管理
│   └── ...
├── engine/               # 测试引擎
│   ├── engine.go         # 核心测试逻辑
│   └── engine_api.go     # API 集成
├── model/                # 模型适配器
│   ├── model.go          # 接口定义
│   ├── openai.go         # OpenAI 实现
│   ├── anthropic.go      # Anthropic 实现
│   └── gemini.go         # Gemini 实现
├── config/               # 配置管理
├── report/               # 报告生成
├── web/                  # 前端项目
│   ├── src/
│   │   ├── views/        # 页面组件
│   │   ├── components/   # 可复用组件
│   │   ├── api/          # API 调用
│   │   └── stores/       # 状态管理
│   └── ...
├── data/                 # 数据目录
│   └── machines/         # 机器配置存储
├── dataset/              # 测试数据集
├── build.sh              # 构建脚本
├── start.bat             # 启动脚本
├── Makefile              # Make 配置
└── config.yaml           # 默认配置
```

## 常见问题

### 1. 如何添加新的模型？

在 `config.yaml` 中添加模型配置，或通过 Web 界面的"后端管理"添加。

### 2. 如何使用数据集进行批量测试？

创建数据集文件（每行一个问题），在配置中设置 `dataset_path`：
```yaml
prompt:
  dataset_path: "dataset/prompts.txt"
```

### 3. 测试时遇到超时怎么办？

增加 `request_timeout` 值：
```yaml
test:
  request_timeout: 10m  # 10 分钟
```

### 4. 如何配置代理？

在配置文件中添加代理，并在模型中引用：
```yaml
proxies:
  - name: "my-proxy"
    url: "http://proxy.example.com:8080"

models:
  - name: "gpt-4"
    proxy_name: "my-proxy"
```

### 5. Web 界面端口被占用怎么办？

使用 `-p` 参数指定其他端口：
```bash
./llm-test-server -p 9000
```

### 6. 如何跳过某个模型？

在模型配置中设置 `skip: true`：
```yaml
models:
  - name: "slow-model"
    skip: true
```

## 性能优化建议

1. **合理设置并发度**: 根据 API 限流策略设置，避免触发限流
2. **使用预热时间**: 首次测试建议设置 5-10 秒预热
3. **启用流式输出**: 对于长文本生成，提升响应体验
4. **批量测试**: 使用数据集获得更准确的性能数据
5. **监控 Prefill 指标**: 关注首 Token 延迟，优化用户体验

## 贡献

欢迎贡献代码、报告问题或提出改进建议。请遵循以下步骤：

1. 将仓库复制到自己的账号
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## 许可证

本项目采用 MIT 许可证 - 详情请参阅 [LICENSE](LICENSE) 文件。

## 相关链接

- **Web 界面文档**: [README_WEB.md](README_WEB.md)
- **GitHub 仓库**: https://github.com/1402366912/llmtest-enhanced
- **问题反馈**: https://github.com/1402366912/llmtest-enhanced/issues
