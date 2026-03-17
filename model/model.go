package model

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lemonlinger/llm-test/config"
	"github.com/sugarme/tokenizer"
)

// contextKey 用于上下文传值
type contextKey string

// 上下文键定义
const (
	ProxyURLContextKey contextKey = "proxy_url"
	StopAfterFirstTokenContextKey contextKey = "stop_after_first_token"
)

// WithStopAfterFirstToken 标记“收到首个有效token后即可提前结束流式读取”。
// 主要用于 warmup：避免生成完整输出带来的耗时与噪声。
func WithStopAfterFirstToken(ctx context.Context) context.Context {
	return context.WithValue(ctx, StopAfterFirstTokenContextKey, true)
}

// ShouldStopAfterFirstToken 返回是否启用了“首token后提前结束”。
func ShouldStopAfterFirstToken(ctx context.Context) bool {
	v := ctx.Value(StopAfterFirstTokenContextKey)
	b, ok := v.(bool)
	return ok && b
}

// LLMResponse 定义模型响应结构
type LLMResponse struct {
	Content      string
	InputTokens  int
	OutputTokens int
	// 流式响应专用指标
	TimeToFirstToken time.Duration // 首个token的响应时间
	TokensPerSecond  float64       // 流式响应的token生成速率
}

// LLMModel 定义大语言模型接口
type LLMModel interface {
	// 获取模型名称
	GetName() string
	// 生成响应
	GenerateResponse(ctx context.Context, systemMessage, userMessage string, stream bool) (*LLMResponse, error)
	// 生成多模态响应（包含图片）
	GenerateVisionResponse(ctx context.Context, systemMessage, userMessage, imagePath string, stream bool) (*LLMResponse, error)
	// 获取模型特定的并发度配置
	GetConcurrencyLevels() []int
	// 获取模型特定的流式输出设置
	GetStreamSetting() *bool
	// 获取模型使用的代理名称
	GetProxyName() string
	// 是否支持多模态
	SupportsVision() bool
}

// 初始化所有配置的模型
func InitializeModels(modelConfigs []config.ModelConfig, proxies []config.ProxyConfig) ([]LLMModel, error) {
	models := make([]LLMModel, 0, len(modelConfigs))

	for _, cfg := range modelConfigs {
		if cfg.Skip {
			continue
		}

		var model LLMModel
		var err error

		switch cfg.Type {
		case "openai":
			model, err = NewOpenAIModel(cfg, proxies)
		case "anthropic":
			model, err = NewAnthropicModel(cfg, proxies)
		case "gemini":
			model, err = NewGeminiModel(cfg, proxies)
		default:
			return nil, fmt.Errorf("不支持的模型类型: %s", cfg.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("初始化模型 %s 失败: %w", cfg.Name, err)
		}

		models = append(models, model)
	}

	return models, nil
}

// BaseModel 提供基本的模型实现
type BaseModel struct {
	config    config.ModelConfig
	tokenizer *tokenizer.Tokenizer // 可选的tokenizer，用于精确token计数
	// tokenizer.json 的路径（即使Go侧加载失败，也保留以便Python兜底）
	tokenizerJSONPath string
	tokenizerVerified bool // 是否已验证tokenizer可用
	tokenizerError    error // tokenizer初始化错误（如果有）
}

// GetName 返回模型名称
func (m *BaseModel) GetName() string {
	return m.config.Name
}

// GetConcurrencyLevels 返回模型特定的并发度配置
func (m *BaseModel) GetConcurrencyLevels() []int {
	return m.config.ConcurrencyLevels
}

// GetStreamSetting 返回模型特定的流式输出设置
func (m *BaseModel) GetStreamSetting() *bool {
	return m.config.Stream
}

// GetProxyName 返回模型使用的代理名称
func (m *BaseModel) GetProxyName() string {
	return m.config.ProxyName
}

// GenerateVisionResponse 默认多模态实现（回退到普通文本模式）
func (m *BaseModel) GenerateVisionResponse(ctx context.Context, systemMessage, userMessage, imagePath string, stream bool) (*LLMResponse, error) {
	// 默认实现：忽略图片，只处理文本
	return nil, fmt.Errorf("模型 %s 不支持多模态功能", m.GetName())
}

// SupportsVision 默认不支持多模态
func (m *BaseModel) SupportsVision() bool {
	return false
}

// LoadTokenizer 尝试加载tokenizer，优先使用TokenizerPath，否则从params.model目录自动查找
func (m *BaseModel) LoadTokenizer() {
	var tokenizerPath string
	
	// 优先使用配置中指定的tokenizer路径
	if m.config.TokenizerPath != "" {
		tokenizerPath = m.config.TokenizerPath
	} else {
		// 尝试从params.model参数推断
		if modelPath, ok := m.config.Params["model"].(string); ok && modelPath != "" {
			// params.model本身就是模型目录，直接在该目录下查找tokenizer.json
			tokenizerPath = filepath.Join(modelPath, "tokenizer.json")
		}
	}
	
	// 如果找到了tokenizer路径，尝试加载
	if tokenizerPath != "" {
		// 记录路径，便于Python兜底
		m.tokenizerJSONPath = tokenizerPath

		// 不再尝试用Go库加载，统一使用Python计数，避免Go正则不兼容
		log.Printf("将使用 Python tokenizers 进行计数: %s", tokenizerPath)
		m.tokenizer = nil
	} else {
		log.Printf("未配置tokenizer路径，将使用估算方法计算token")
	}
}

// countTokensViaPython 使用Python的HF tokenizers精确计算
func (m *BaseModel) countTokensViaPython(text string) (int, error) {
	if m.tokenizerJSONPath == "" {
		return 0, fmt.Errorf("tokenizer json path empty")
	}

	// 解析脚本路径：与可执行文件同目录下的 tools/token_count.py
	exePath, err := os.Executable()
	if err != nil {
		return 0, err
	}
	exeDir := filepath.Dir(exePath)
	scriptPath := filepath.Join(exeDir, "tools", "token_count.py")
	if _, err := os.Stat(scriptPath); err != nil {
		// 尝试工作目录
		if _, err2 := os.Stat(filepath.Join("tools", "token_count.py")); err2 == nil {
			scriptPath = filepath.Join("tools", "token_count.py")
		} else {
			return 0, fmt.Errorf("token_count.py not found")
		}
	}

	// 选择python命令
	py := getPythonCommand()

	cmd := exec.Command(py, scriptPath, m.tokenizerJSONPath)
	cmd.Stdin = strings.NewReader(text)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("警告: Python tokenizer 调用失败 (cmd=%s %s %s): %v, stderr=%s", py, scriptPath, m.tokenizerJSONPath, err, stderr.String())
		return 0, fmt.Errorf("python run error: %v, stderr=%s", err, stderr.String())
	}

	s := strings.TrimSpace(out.String())
	if s == "" {
		return 0, fmt.Errorf("empty output from python (input text len=%d)", len(text))
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer from python: %q (stderr=%s)", s, stderr.String())
	}
	return n, nil
}

// CountTokens 使用tokenizer精确计算token数，失败时报错
func (m *BaseModel) CountTokens(text string) int {
	// 如果文本为空，直接返回0，不调用Python
	if text == "" {
		return 0
	}
	
	// 优先使用 Python tokenizers（与HF完全一致）
	n, err := m.countTokensViaPython(text)
	if err == nil {
		// 第一次成功时记录日志
		if !m.tokenizerVerified {
			log.Printf("✓ 成功使用 Python tokenizer: %s", m.tokenizerJSONPath)
			m.tokenizerVerified = true
		}
		// 如果返回0但文本不为空，打印警告
		if n == 0 && len(text) > 0 {
			log.Printf("警告: tokenizer 返回0，但文本长度为 %d 字符，文本前100字符: %s", len(text), truncateText(text, 100))
		}
		return n
	}
	
	// Python tokenizer 失败，记录错误并终止
	m.tokenizerError = err
	log.Fatalf("错误: tokenizer 不可用，无法精确计数 token。\n"+
		"原因: %v\n"+
		"tokenizer 路径: %s\n"+
		"请确保:\n"+
		"  1. 已安装 Python tokenizers 库 (pip install tokenizers)\n"+
		"  2. tokenizer.json 路径正确\n"+
		"  3. Python 命令可用 (%s)",
		err, m.tokenizerJSONPath, getPythonCommand())
	return 0 // 不会执行到这里
}

// getPythonCommand 返回当前平台的 Python 命令
func getPythonCommand() string {
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

// truncateText 截断文本到指定长度
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}
