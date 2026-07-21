.PHONY: all build clean run prefill-bench help

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

# HTML 等价的隔离 Prefill/Decode 基准。
# 示例: make prefill-bench PREFILL_ARGS="--url http://127.0.0.1:60015 --lengths 10000,50000"
prefill-bench:
	@python3 tools/prefill_bench.py $(PREFILL_ARGS)

# 帮助
help:
	@echo "LLM-Test 构建命令："
	@echo ""
	@echo "  make build    - 构建后端"
	@echo "  make run      - 运行服务器"
	@echo "  make prefill-bench PREFILL_ARGS=\"...\" - 隔离 Prefill/Decode 基准"
	@echo "  make clean    - 清理构建产物"
	@echo ""
	@echo "快速开始："
	@echo "  make build && make run"
