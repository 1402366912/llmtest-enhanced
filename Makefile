.PHONY: all build clean run help

# 默认目标
all: build

# 构建后端
build:
	@echo ">>> 构建后端..."
	@go build -o llm-test-server ./cmd/server
	@echo "✓ 构建完成！运行: ./llm-test-server"

# 清理构建产物
clean:
	@rm -rf llm-test-server llm-test-server.exe
	@echo "✓ 清理完成"

# 运行服务器
run:
	@./llm-test-server -config config.yaml -addr :8088

# 帮助
help:
	@echo "LLM-Test 构建命令："
	@echo ""
	@echo "  make build    - 构建后端"
	@echo "  make run      - 运行服务器"
	@echo "  make clean    - 清理构建产物"
	@echo ""
	@echo "快速开始："
	@echo "  make build && make run"
