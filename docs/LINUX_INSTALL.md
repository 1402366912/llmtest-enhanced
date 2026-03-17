# LLM-Test Linux 完整安装指南

本文档详细介绍如何在 Linux 系统上从零开始安装和运行 LLM-Test。

## 目录

1. [环境要求](#环境要求)
2. [安装 Node.js 20+](#安装-nodejs-20)
3. [安装 Go 语言](#安装-go-语言)
4. [下载项目](#下载项目)
5. [编译项目](#编译项目)
6. [配置说明](#配置说明)
7. [运行服务](#运行服务)
8. [常见问题](#常见问题)

---

## 环境要求

| 软件 | 最低版本 | 说明 |
|-----|---------|------|
| Node.js | 20.19+ | 用于构建前端 |
| npm | 10+ | Node.js 包管理器 |
| Go | 1.18+ | 用于编译后端 |
| Git | 任意 | 用于下载代码 |

---

## 安装 Node.js 20+

### 方法一：使用 nvm（推荐）

nvm 可以方便地管理多个 Node.js 版本。

```bash
# 1. 安装 nvm
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash

# 2. 重新加载 shell 配置
source ~/.bashrc
# 或者
source ~/.zshrc

# 3. 验证 nvm 安装
nvm --version

# 4. 安装 Node.js 20
nvm install 20

# 5. 设置默认版本
nvm alias default 20

# 6. 验证安装
node -v   # 应显示 v20.x.x
npm -v    # 应显示 10.x.x
```

### 方法二：使用 NodeSource 官方源

```bash
# Ubuntu / Debian
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs

# CentOS / RHEL / Fedora
curl -fsSL https://rpm.nodesource.com/setup_20.x | sudo bash -
sudo yum install -y nodejs

# 验证安装
node -v
npm -v
```

### 方法三：手动安装（适用于无 sudo 权限）

```bash
# 1. 下载 Node.js 二进制包
cd ~
wget https://nodejs.org/dist/v20.10.0/node-v20.10.0-linux-x64.tar.xz

# 2. 解压
tar -xJf node-v20.10.0-linux-x64.tar.xz

# 3. 添加到 PATH
echo 'export PATH=$HOME/node-v20.10.0-linux-x64/bin:$PATH' >> ~/.bashrc
source ~/.bashrc

# 4. 验证
node -v
npm -v
```

---

## 安装 Go 语言

### 方法一：官方安装包（推荐）

```bash
# 1. 下载 Go
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz

# 2. 解压到 /usr/local（需要 sudo）
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# 3. 添加到 PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# 4. 验证
go version
```

### 方法二：使用包管理器

```bash
# Ubuntu 22.04+
sudo apt update
sudo apt install -y golang-go

# 验证
go version
```

### 方法三：无 sudo 权限安装

```bash
# 1. 下载并解压到用户目录
cd ~
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
tar -xzf go1.21.5.linux-amd64.tar.gz

# 2. 配置环境变量
echo 'export GOROOT=$HOME/go' >> ~/.bashrc
echo 'export GOPATH=$HOME/gopath' >> ~/.bashrc
echo 'export PATH=$PATH:$GOROOT/bin:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# 3. 验证
go version
```

---

## 下载项目

```bash
# 方式一：Git 克隆
git clone https://github.com/lemonlinger/llm-test.git
cd llm-test

# 方式二：下载 ZIP
wget https://github.com/lemonlinger/llm-test/archive/refs/heads/main.zip
unzip main.zip
cd llm-test-main
```

---

## 编译项目

### 自动编译（推荐）

```bash
# 1. 添加执行权限
chmod +x build.sh

# 2. 运行编译脚本
./build.sh
```

### 手动编译

如果自动编译失败，按以下步骤手动操作：

```bash
# ===== 步骤 1：编译前端 =====

cd web

# 安装依赖
npm install

# ⚠️ 重要：添加执行权限（解决 Permission denied 错误）
chmod +x node_modules/.bin/*

# 编译前端
npm run build

# 返回项目根目录
cd ..

# ===== 步骤 2：编译后端 =====

# 编译 Go 程序
go build -o llm-test-server ./cmd/server/main.go

# 添加执行权限
chmod +x llm-test-server

# ===== 步骤 3：验证编译结果 =====

# 检查文件是否存在
ls -la llm-test-server
ls -la api/static/dist/index.html
```

### 权限问题汇总

```bash
# 如果遇到 "Permission denied" 错误，执行以下命令：

# 给 node_modules 可执行文件添加权限
chmod +x web/node_modules/.bin/*

# 给编译后的服务器添加权限
chmod +x llm-test-server

# 给脚本添加权限
chmod +x build.sh

# 给数据目录添加写权限（如果需要）
chmod -R 755 data/
chmod -R 755 dataset/
```

---

## 配置说明

### 编辑配置文件

```bash
# 使用 vim 或 nano 编辑
vim config.yaml
# 或
nano config.yaml
```

### 最小配置示例

```yaml
test:
  concurrency: 1
  stress_test_mode: false
  duration: 30s
  request_timeout: 2m
  concurrency_levels:
    - 1
    - 10
    - 20
  context_token_start: 128
  context_token_end: 4096

models:
  - name: my-model
    type: openai
    api_key: "your-api-key"
    base_url: http://localhost:8080/v1    # 你的 LLM 服务地址
    params:
      model: your-model-name
      max_tokens: 4096
      temperature: 0.7
    skip: false

prompt:
  system_message: 你是一个助手，请提供回答。
  dataset_path: dataset/prompts.txt
  stream: true
```

### 配置项说明

| 配置项 | 说明 |
|-------|------|
| `stress_test_mode` | `false`=请求数模式（并发×3个请求），`true`=压力测试模式（按时间） |
| `concurrency_levels` | 要测试的并发级别列表 |
| `context_token_start/end` | 上下文长度测试范围 |
| `base_url` | LLM API 地址（vLLM/SGLang/LMDeploy 等） |
| `dataset_path` | 测试数据集路径，随机抽取题目 |
| `stream` | 是否使用流式输出（推荐开启） |

---

## 运行服务

### 方式一：前台运行（测试用）

```bash
./llm-test-server -config config.yaml -addr :9090
```

### 方式二：后台运行

```bash
# 启动
nohup ./llm-test-server -config config.yaml -addr :9090 > server.log 2>&1 &

# 查看日志
tail -f server.log

# 查看进程
ps aux | grep llm-test-server

# 停止服务
pkill llm-test-server
```

### 方式三：使用 systemd（生产环境推荐）

```bash
# 1. 创建服务文件
sudo vim /etc/systemd/system/llm-test.service
```

写入以下内容（注意修改路径和用户名）：

```ini
[Unit]
Description=LLM Performance Test Server
After=network.target

[Service]
Type=simple
User=你的用户名
WorkingDirectory=/home/你的用户名/llm-test
ExecStart=/home/你的用户名/llm-test/llm-test-server -config config.yaml -addr :9090
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# 2. 启用并启动服务
sudo systemctl daemon-reload
sudo systemctl enable llm-test
sudo systemctl start llm-test

# 3. 查看状态
sudo systemctl status llm-test

# 4. 查看日志
sudo journalctl -u llm-test -f

# 5. 重启/停止
sudo systemctl restart llm-test
sudo systemctl stop llm-test
```

### 访问 Web 界面

```bash
# 本地访问
http://localhost:9090

# 远程访问（需要开放防火墙）
# Ubuntu/Debian
sudo ufw allow 9090

# CentOS/RHEL
sudo firewall-cmd --add-port=9090/tcp --permanent
sudo firewall-cmd --reload

# 然后访问
http://你的服务器IP:9090
```

---

## 常见问题

### Q1: `vue-tsc: Permission denied`

```bash
# 解决方案：添加执行权限
chmod +x web/node_modules/.bin/*
```

### Q2: `Vite requires Node.js version 20.19+`

```bash
# 解决方案：升级 Node.js
# 使用 nvm
nvm install 20
nvm use 20

# 或使用 NodeSource
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
```

### Q3: `crypto.hash is not a function`

```bash
# 这是 Node.js 版本过低导致的，需要升级到 20+
node -v  # 检查版本
# 如果低于 20，参考上面的升级方法
```

### Q4: `go: command not found`

```bash
# 检查 PATH 配置
echo $PATH

# 重新加载配置
source ~/.bashrc

# 如果还是不行，检查 Go 安装路径
ls -la /usr/local/go/bin/go
```

### Q5: 端口被占用

```bash
# 查看端口占用
netstat -tlnp | grep 9090
# 或
ss -tlnp | grep 9090

# 杀死占用进程
kill -9 进程PID

# 或使用其他端口
./llm-test-server -config config.yaml -addr :8088
```

### Q6: 无法访问 Web 界面

```bash
# 1. 检查服务是否运行
ps aux | grep llm-test-server

# 2. 检查端口监听
netstat -tlnp | grep 9090

# 3. 检查防火墙
sudo ufw status
# 或
sudo firewall-cmd --list-all

# 4. 检查日志
tail -100 server.log
```

### Q7: 测试数据保存在哪里？

```bash
# 数据目录结构
data/
├── machines.json              # 机器列表
├── {machine_id}/              # 每台机器的数据
│   ├── backends.json          # 后端列表
│   └── {backend_id}/          # 每个后端的数据
│       ├── config.yaml        # 后端配置
│       └── results/           # 测试结果
│           ├── xxx.json
│           └── xxx.xlsx
```

---

## 快速启动命令汇总

```bash
# 一键安装 Node.js 20（Ubuntu/Debian）
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash - && sudo apt install -y nodejs

# 一键安装 Go
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz && sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz && echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc && source ~/.bashrc

# 编译项目
chmod +x build.sh && ./build.sh

# 启动服务
./llm-test-server -config config.yaml -addr :9090
```

---

## 联系与反馈

如有问题，请提交 Issue 或联系开发者。



# 进入 web 目录
cd ~/桌面/llm-test./web

# 给所有 bin 文件添加执行权限
chmod +x node_modules/.bin/*

# 重新构建
npm run build