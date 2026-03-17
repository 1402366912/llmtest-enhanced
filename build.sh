#!/bin/bash

echo "========================================"
echo "  LLM-Test 构建脚本 (Linux/macOS)"
echo "========================================"

set -e

echo ""
echo "[1/3] 安装前端依赖..."
cd web
npm install

echo ""
echo "[2/3] 构建前端..."
npm run build
cd ..

echo ""
echo "[3/3] 构建后端..."
go build -o llm-test-server ./cmd/server

echo ""
echo "========================================"
echo "  构建完成！"
echo "========================================"
echo ""
echo "启动命令:"
echo "  ./llm-test-server"
echo ""
echo "然后访问: http://localhost:8080"
echo ""
