# LLM-Test Web 界面使用指南

LLM-Test 提供了功能强大的 Web 界面，支持可视化配置管理、多机器管理、实时测试监控和性能对比分析。

## 快速开始

### 方式一：一键启动（推荐）

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

启动后访问：http://localhost:8080

### 方式二：使用 Make（Linux/macOS）

```bash
make build && make run
```

### 方式三：开发模式

适合前端开发，支持热重载。需要两个终端：

```bash
# 终端1 - 启动后端服务器
go run ./cmd/server -c config.yaml -p 8088

# 终端2 - 启动前端开发服务器
cd web
npm install
npm run dev
```

开发模式访问：http://localhost:5173

## 核心功能

### 1. 多机器管理
- 创建和管理多个测试机器配置
- 每个机器独立的配置文件和测试环境
- 快速切换不同机器进行测试
- 支持机器的导入/导出

### 2. 后端管理
- 添加/编辑/删除 LLM 后端配置
- 支持 OpenAI、Anthropic、Gemini 等多种模型类型
- 配置 API Key、Base URL、代理等参数
- 启用/禁用特定后端
- 为每个后端设置独立的并发度和测试参数

### 3. 测试配置
- 可视化编辑测试参数：
  - 并发数（支持多级并发测试）
  - 测试持续时间
  - 预热时间
  - 请求超时
  - 重试次数
- 提示词配置：
  - 系统消息
  - 用户消息
  - 数据集路径（支持批量测试）
  - 流式输出开关

### 4. 实时测试监控
- 一键启动/停止测试
- WebSocket 实时推送测试进度
- 实时显示关键指标：
  - 当前测试模型和并发度
  - 成功率和失败率
  - RPS（每秒请求数）
  - TPS（每秒 Token 数）
  - 平均延迟
  - Token 使用统计

### 5. 结果分析与可视化
- 详细的测试结果表格
- 支持多模型、多并发度对比
- 性能天梯图（ECharts 可视化）
- 延迟百分位数统计（P50、P90、P95、P99）
- 导出测试报告（支持 Excel、JSON 格式）

### 6. 历史记录
- 保存所有测试历史
- 按时间、机器、模型筛选
- 对比不同时间点的测试结果
- 导出历史数据

## 使用教程

### 第一步：创建机器

1. 访问 Web 界面首页
2. 点击"创建新机器"按钮
3. 输入机器名称（如：测试环境1）
4. 系统会自动创建独立的配置文件

### 第二步：配置后端

1. 在机器管理页面，点击"后端管理"
2. 点击"添加后端"按钮
3. 填写后端信息：
   - 名称：给后端起个名字（如：GPT-4）
   - 类型：选择模型类型（openai/anthropic/gemini）
   - API Key：填入你的 API 密钥
   - Base URL：API 端点地址
   - 模型 ID：具体的模型标识符
4. 配置模型参数：
   - Temperature：控制输出随机性（0-1）
   - Max Tokens：最大输出长度
   - 并发度：该模型的测试并发数
5. 如需代理，填写代理配置
6. 点击"保存"

### 第三步：配置测试参数

1. 在"测试配置"页面设置：
   - 并发级别：如 [5, 10, 20]，会依次测试这些并发度
   - 测试时长：每个并发级别的测试持续时间
   - 预热时间：正式测试前的预热时长
   - 请求超时：单个请求的超时时间
   - 重试次数：失败后的重试次数

2. 配置提示词：
   - 系统消息：定义 AI 的角色和行为
   - 用户消息：测试用的问题
   - 或使用数据集：指定数据集文件路径进行批量测试

3. 选择是否启用流式输出

### 第四步：启动测试

1. 确认所有配置无误
2. 点击"开始测试"按钮
3. 实时查看测试进度：
   - 当前测试的模型和并发度
   - 成功/失败请求数
   - 实时 RPS 和 TPS
   - 平均延迟

### 第五步：查看结果

测试完成后，可以：
- 在结果表格中查看详细数据
- 查看性能天梯图对比不同模型
- 导出 Excel 或 JSON 格式报告
- 在历史记录中查看过往测试

## API 接口文档

### 机器管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/machines | 获取所有机器列表 |
| POST | /api/machines | 创建新机器 |
| DELETE | /api/machines/:id | 删除机器 |
| POST | /api/machines/:id/switch | 切换到指定机器 |

### 后端管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/backends | 获取后端列表 |
| POST | /api/backends | 添加后端 |
| PUT | /api/backends/:name | 更新后端 |
| DELETE | /api/backends/:name | 删除后端 |
| PUT | /api/backends/:name/toggle | 切换后端启用状态 |

### 配置管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/config | 获取当前配置 |
| PUT | /api/config | 更新配置 |
| POST | /api/config/reload | 重新加载配置文件 |

### 测试控制
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/test/start | 启动测试 |
| POST | /api/test/stop | 停止测试 |
| GET | /api/test/status | 获取测试状态 |
| GET | /api/test/results | 获取测试结果 |
| WS | /api/ws | WebSocket 实时进度推送 |

### 历史记录
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/history | 获取测试历史 |
| GET | /api/history/:id | 获取指定历史记录 |
| DELETE | /api/history/:id | 删除历史记录 |

### 数据导出
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/export/xlsx | 导出 Excel 报告 |
| GET | /api/export/json | 导出 JSON 数据 |

## 技术栈

### 后端
- **语言**: Go 1.18+
- **Web 框架**: Gin
- **实时通信**: Gorilla WebSocket
- **配置管理**: YAML
- **并发控制**: Go Goroutines + Context

### 前端
- **框架**: Vue 3 (Composition API)
- **UI 库**: Naive UI
- **图表**: ECharts + vue-echarts
- **状态管理**: Pinia
- **HTTP 客户端**: Axios
- **构建工具**: Vite
- **语言**: TypeScript

### 通信协议
- **RESTful API**: 配置管理、后端管理
- **WebSocket**: 实时测试进度推送

## 项目结构

```
llm-test/
├── cmd/
│   └── server/
│       └── main.go              # 服务器入口
├── api/
│   ├── server.go                # 服务器核心
│   ├── test.go                  # 测试控制接口
│   ├── websocket.go             # WebSocket 处理
│   ├── config_save.go           # 配置保存
│   ├── machines.go              # 机器管理
│   ├── backends.go              # 后端管理
│   ├── history.go               # 历史记录
│   ├── xlsx.go                  # Excel 导出
│   ├── data_transfer.go         # 数据传输
│   └── static.go                # 静态文件服务
├── engine/
│   ├── engine.go                # 测试引擎核心
│   └── engine_api.go            # 带进度回调的测试引擎
├── model/
│   ├── model.go                 # 模型接口定义
│   ├── openai.go                # OpenAI 模型实现
│   ├── anthropic.go             # Anthropic 模型实现
│   └── gemini.go                # Gemini 模型实现
├── config/
│   └── config.go                # 配置结构定义
├── report/
│   └── report.go                # 报告生成
├── web/                         # 前端项目
│   ├── src/
│   │   ├── main.ts              # 入口文件
│   │   ├── App.vue              # 根组件
│   │   ├── api/                 # API 调用封装
│   │   ├── components/          # 可复用组件
│   │   ├── views/               # 页面视图
│   │   ├── stores/              # Pinia 状态管理
│   │   └── types/               # TypeScript 类型定义
│   ├── package.json
│   └── vite.config.ts
├── data/                        # 机器数据目录
│   └── machines/
│       └── [machine-id]/
│           └── config.yaml      # 机器配置文件
├── dataset/                     # 测试数据集
├── build.sh                     # Linux/macOS 构建脚本
├── build.bat                    # Windows 构建脚本
├── start.bat                    # Windows 启动脚本
├── Makefile                     # Make 构建配置
├── config.yaml                  # 默认配置文件
├── main.go                      # CLI 入口
└── README.md                    # 主文档
```

## 常见问题

### 1. 构建失败

**问题**: npm install 或 go build 失败

**解决方案**:
- 确保 Node.js 18+ 和 Go 1.18+ 已安装
- 检查网络连接，可能需要配置代理
- 清理缓存后重试：
  ```bash
  cd web && rm -rf node_modules package-lock.json
  npm install
  ```

### 2. 端口被占用

**问题**: 启动时提示端口 8080 或 8088 被占用

**解决方案**:
```bash
# 指定其他端口
./llm-test-server -addr :9000

# 或使用简写
./llm-test-server -p 9000
```

### 3. WebSocket 连接失败

**问题**: 测试进度不更新

**解决方案**:
- 检查浏览器控制台是否有 WebSocket 错误
- 确认后端服务正常运行
- 检查防火墙设置
- 刷新页面重新连接

### 4. API 请求失败

**问题**: 测试时提示 API 错误

**解决方案**:
- 检查 API Key 是否正确
- 验证 Base URL 是否可访问
- 如需代理，确认代理配置正确
- 查看后端日志获取详细错误信息

### 5. 前端开发模式跨域问题

**问题**: 开发模式下 API 请求被 CORS 阻止

**解决方案**:
- 后端已配置 CORS，允许所有来源
- 检查 `web/vite.config.ts` 中的代理配置
- 确保后端运行在 8088 端口

## 高级配置

### 自定义数据集

创建 `dataset/my-dataset.txt` 文件，每行一个测试问题：
```
请解释量子计算的基本原理
什么是机器学习？
介绍一下深度学习的应用
```

在 Web 界面的提示词配置中，设置数据集路径为 `dataset/my-dataset.txt`。

### 配置代理

在后端配置中添加代理：
```yaml
proxies:
  - name: "my-proxy"
    url: "http://proxy.example.com:8080"
```

然后在后端配置中引用：
```yaml
models:
  - name: "gpt-4"
    proxy_name: "my-proxy"
```

### 多机器隔离

每个机器的配置文件独立存储在 `data/machines/[machine-id]/config.yaml`，互不影响。可以：
- 为不同环境创建不同机器（开发、测试、生产）
- 为不同项目创建独立配置
- 快速切换测试场景

## 性能优化建议

1. **并发度设置**: 根据 API 限流策略合理设置，避免触发限流
2. **预热时间**: 首次测试建议设置 5-10 秒预热，让连接池预热
3. **超时时间**: 根据模型响应速度调整，避免过短导致误判
4. **流式输出**: 对于长文本生成，启用流式输出可提升用户体验
5. **批量测试**: 使用数据集进行批量测试，获得更准确的性能数据

## 贡献指南

欢迎贡献代码、报告问题或提出改进建议：

1. 将仓库复制到自己的账号
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## 许可证

本项目采用 MIT 许可证 - 详情请参阅 [LICENSE](LICENSE) 文件。

## 相关链接

- [主文档](README.md)
- [配置文件示例](config.yaml)
- [GitHub 仓库](https://github.com/1402366912/llmtest-enhanced)


## 单请求基准（v2.2）

Web 界面的“单请求基准”页完整内嵌修复过 `delta.reasoning` 计时的《本地大模型推理速度测试工具 v2.2》，可做输入长度扫描、并发测试、图表、浏览器历史与导出。选择 1Cat 模型并点“使用当前模型配置”，会填入聊天接口、实际模型 ID 和 API Key。若模型地址配置为 `localhost`，页面会改用当前浏览器访问的服务器主机名，端口保持不变；浏览器仍需能连通模型端口，且模型服务允许跨域请求。也可以在工具内手动填写其他地址。

单请求工具在浏览器中直接计时，历史保存在浏览器 localStorage；主测试页的持续压测由 Go 后端执行，结果保存在服务端。两种结果使用不同负载与计时口径，比较前应核对输入 token 数、TTFT、输出 token 数、并发度和前缀缓存命中情况。

单请求工具原作者项目：[gengchaogit/llm_speedtest](https://github.com/gengchaogit/llm_speedtest)。集成保留了 v2.2 的完整 HTML 功能，修复版本来源为本机已有的 `本地大模型推理速度测试工具v2.2.html`。
