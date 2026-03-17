package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration 自定义 Duration 类型，支持 JSON 字符串格式
type Duration time.Duration

// UnmarshalJSON 实现 JSON 反序列化
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// 如果不是字符串，尝试解析为数字（纳秒）
		var n int64
		if err2 := json.Unmarshal(data, &n); err2 != nil {
			return err
		}
		*d = Duration(time.Duration(n))
		return nil
	}
	
	// 解析字符串格式（如 "30s", "1m" 等）
	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("无法解析持续时间 %s: %w", s, err)
	}
	*d = Duration(duration)
	return nil
}

// MarshalJSON 实现 JSON 序列化
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalYAML 实现 YAML 反序列化
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		// 如果不是字符串，尝试解析为数字
		var n int64
		if err2 := unmarshal(&n); err2 != nil {
			return err
		}
		*d = Duration(time.Duration(n))
		return nil
	}
	
	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("无法解析持续时间 %s: %w", s, err)
	}
	*d = Duration(duration)
	return nil
}

// MarshalYAML 实现 YAML 序列化
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Duration 返回 time.Duration
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String 返回字符串表示
func (d Duration) String() string {
	return time.Duration(d).String()
}

// Config 定义整体配置结构
type Config struct {
	// 测试配置
	Test TestConfig `yaml:"test" json:"test"`
	// 模型配置列表
	Models []ModelConfig `yaml:"models" json:"models"`
	// 测试提示词
	Prompt PromptConfig `yaml:"prompt" json:"prompt"`
	// 代理配置列表
	Proxies []ProxyConfig `yaml:"proxies" json:"proxies"`
}

// TestConfig 定义测试相关配置
type TestConfig struct {
	// 并发数
	Concurrency int `yaml:"concurrency" json:"concurrency"`
	// 压力测试模式（true: 按时间测试，false: 按请求数测试，请求数=并发×3）
	StressTestMode bool `yaml:"stress_test_mode" json:"stress_test_mode"`
	// 测试持续时间（仅在 StressTestMode=true 时使用）
	Duration Duration `yaml:"duration" json:"duration"`
	// 每个并发度的预热时间
	WarmupDuration Duration `yaml:"warmup_duration" json:"warmup_duration"`
	// Warmup 请求次数（每模型一次 warmup 的请求数；0 表示关闭 warmup 请求）
	WarmupRequests int `yaml:"warmup_requests,omitempty" json:"warmup_requests,omitempty"`
	// Warmup 请求超时时间（用于首 token 退出/超时退出的 warmup 机制）
	WarmupTimeout Duration `yaml:"warmup_timeout,omitempty" json:"warmup_timeout,omitempty"`
	// 每个请求的超时时间
	RequestTimeout Duration `yaml:"request_timeout" json:"request_timeout"`
	// 递增的并发数列表，如果为空则只使用 Concurrency
	ConcurrencyLevels []int `yaml:"concurrency_levels" json:"concurrency_levels"`
	// 是否显示进度条
	ShowProgress bool `yaml:"show_progress" json:"show_progress"`
	// 重试次数
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
	// 需要计算的延迟百分位列表，例如 [50, 90, 95, 99]
	LatencyPercentiles []int `yaml:"latency_percentiles" json:"latency_percentiles"`
	// 目标上下文长度（Token）梯度列表；为空则不进行上下文长度分组测试
	ContextTokenLevels []int `yaml:"context_token_levels,omitempty" json:"context_token_levels,omitempty"`
	// 上下文起始长度（Token），用于自动生成梯度，如果ContextTokenLevels为空则使用此值
	ContextTokenStart int `yaml:"context_token_start,omitempty" json:"context_token_start,omitempty"`
	// 上下文结束长度（Token），用于自动生成梯度，如果ContextTokenLevels为空则使用此值
	ContextTokenEnd int `yaml:"context_token_end,omitempty" json:"context_token_end,omitempty"`
	// 上下文长度近似控制的容差（占比，例如0.1表示±10%），用于未来可能的校准逻辑
	ContextTolerance float64 `yaml:"context_tolerance,omitempty" json:"context_tolerance,omitempty"`
	// 是否启用prefill/decoding阶段指标统计（依赖流式输出才能精确TTFT）
	EnablePrefillMetrics bool `yaml:"enable_prefill_metrics,omitempty" json:"enable_prefill_metrics,omitempty"`
}

// ModelConfig 定义模型相关配置
type ModelConfig struct {
	// 模型名称
	Name string `yaml:"name" json:"name"`
	// 模型类型 (openai, anthropic, gemini等)
	Type string `yaml:"type" json:"type"`
	// API密钥
	APIKey string `yaml:"api_key" json:"api_key"`
	// API基础URL
	BaseURL string `yaml:"base_url" json:"base_url"`
	// 模型参数
	Params map[string]interface{} `yaml:"params" json:"params"`
	// Tokenizer文件路径（可选），如果未指定则自动从params.model目录查找tokenizer.json
	TokenizerPath string `yaml:"tokenizer_path,omitempty" json:"tokenizer_path,omitempty"`
	// 是否跳过该模型
	Skip bool `yaml:"skip" json:"skip"`
	// 模型特定的并发度设置，如果不为空则覆盖全局设置
	ConcurrencyLevels []int `yaml:"concurrency_levels,omitempty" json:"concurrency_levels,omitempty"`
	// 是否启用流式输出，如果未设置则使用全局prompt.stream
	Stream *bool `yaml:"stream,omitempty" json:"stream,omitempty"`
	// 使用的代理名称，如果为空则不使用代理
	ProxyName string `yaml:"proxy_name,omitempty" json:"proxy_name,omitempty"`
}

// PromptConfig 定义提示词配置
type PromptConfig struct {
	// 系统消息
	SystemMessage string `yaml:"system_message" json:"system_message"`
	// 用户消息（单条固定提示词，如果配置了dataset_path则忽略此项）
	UserMessage string `yaml:"user_message" json:"user_message"`
	// 数据集文件路径（每行一个提示词，测试时随机选取）
	DatasetPath string `yaml:"dataset_path,omitempty" json:"dataset_path,omitempty"`
	// 是否启用流式输出
	Stream bool `yaml:"stream" json:"stream"`
	// 图片路径（用于多模态模型）
	ImagePath string `yaml:"image_path,omitempty" json:"image_path,omitempty"`
	// 图片URL（用于多模态模型）
	ImageURL string `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	// 生成上下文填充的基础模板（可选），为空则使用默认模板
	ContextBase string `yaml:"context_base,omitempty" json:"context_base,omitempty"`
}

// ProxyConfig 定义代理配置
type ProxyConfig struct {
	// 代理名称
	Name string `yaml:"name" json:"name"`
	// 代理URL
	URL string `yaml:"url" json:"url"`
}

// LoadConfig 从文件中加载配置
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if config.Test.Concurrency == 0 {
		config.Test.Concurrency = 1
	}
	// 压力测试模式的默认持续时间
	if config.Test.Duration == 0 {
		config.Test.Duration = Duration(30 * time.Second)
	}
	if config.Test.RequestTimeout == 0 {
		config.Test.RequestTimeout = Duration(60 * time.Second)
	}
	// warmup 请求默认值（向后兼容：未配置时不影响旧行为）
	if config.Test.WarmupRequests == 0 {
		config.Test.WarmupRequests = 1
	}
	if config.Test.WarmupTimeout == 0 {
		config.Test.WarmupTimeout = Duration(10 * time.Second)
	}
	if config.Test.MaxRetries == 0 {
		config.Test.MaxRetries = 3
	}
	if config.Test.ContextTolerance == 0 {
		config.Test.ContextTolerance = 0.1
	}

	// 自动生成上下文长度梯度
	// 如果设置了 start 和 end，则优先使用它们生成梯度（忽略预设的 levels 列表）
	if config.Test.ContextTokenStart > 0 && config.Test.ContextTokenEnd > 0 {
		config.Test.ContextTokenLevels = GenerateContextLevels(config.Test.ContextTokenStart, config.Test.ContextTokenEnd)
	}

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// validateConfig 验证配置是否合法
func validateConfig(config *Config) error {
	if len(config.Models) == 0 {
		return fmt.Errorf("至少需要配置一个模型")
	}

	// 必须有 user_message 或 dataset_path 其中之一
	if config.Prompt.UserMessage == "" && config.Prompt.DatasetPath == "" {
		return fmt.Errorf("用户提示词不能为空（请配置 user_message 或 dataset_path）")
	}

	for i, model := range config.Models {
		if model.Name == "" {
			return fmt.Errorf("模型 #%d 未指定名称", i+1)
		}
		if model.Type == "" {
			return fmt.Errorf("模型 %s 未指定类型", model.Name)
		}
		if model.APIKey == "" {
			return fmt.Errorf("模型 %s 未指定API密钥", model.Name)
		}
	}

	return nil
}

// GenerateContextLevels 根据起始和结束值自动生成上下文长度梯度
// 使用每次×2的递增方式：128, 256, 512, 1024, 2048, 4096, 8192, 16384...
func GenerateContextLevels(start, end int) []int {
	if start <= 0 || end <= 0 || start >= end {
		return []int{}
	}

	levels := []int{}
	current := start
	
	for current <= end {
		levels = append(levels, current)
		current = current * 2 // 每次翻倍
	}
	
	// 确保末尾包含end值（如果不在列表中且大于最后一个值）
	if len(levels) > 0 && levels[len(levels)-1] < end {
		levels = append(levels, end)
	}
	
	return levels
}

// SaveConfig 保存配置到文件
func SaveConfig(config *Config, filePath string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}
