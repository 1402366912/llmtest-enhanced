package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lemonlinger/llm-test/config"
)

// getFloat64 安全地从 interface{} 获取 float64
func getFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

// getInt 安全地从 interface{} 获取 int
func getInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	default:
		return 0
	}
}

// OpenAIModel OpenAI模型实现
type OpenAIModel struct {
	BaseModel
	defaultClient *http.Client
	proxyClients  map[string]*http.Client // 代理名称到对应HTTP客户端的映射
}

// StreamOptions 定义流式响应选项
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// OpenAIRequest 定义OpenAI API请求结构
type OpenAIRequest struct {
	Model         string                 `json:"model"`
	Messages      []OpenAIMessage        `json:"messages"`
	Temperature   float64                `json:"temperature"`
	MaxTokens     int                    `json:"max_tokens"`
	Stream        bool                   `json:"stream,omitempty"`
	StreamOptions *StreamOptions         `json:"stream_options,omitempty"`
	Params        map[string]interface{} `json:"-"`
}

// OpenAIMessage 定义OpenAI消息结构
type OpenAIMessage struct {
	Role    string                 `json:"role"`
	Content []OpenAIMessageContent `json:"content,omitempty"`
	Text    string                 `json:"text,omitempty"`
}

// MarshalJSON 自定义序列化方法
func (m OpenAIMessage) MarshalJSON() ([]byte, error) {
	// 创建一个临时结构体用于序列化
	type Alias OpenAIMessage

	// 确保只有一个字段不为空
	if m.Text != "" && len(m.Content) > 0 {
		return nil, fmt.Errorf("both Text and Content cannot be set")
	}

	if m.Text != "" {
		text := m.Text
		m.Text = ""
		return json.Marshal(&struct {
			*Alias
			Content string `json:"content"`
		}{
			Alias:   (*Alias)(&m),
			Content: text,
		})
	} else if len(m.Content) > 0 {
		return json.Marshal(&struct {
			*Alias
			Content []OpenAIMessageContent `json:"content"`
		}{
			Alias:   (*Alias)(&m),
			Content: m.Content,
		})
	}

	// 如果两个字段都为空，返回空的 JSON 对象
	return json.Marshal(&struct {
		*Alias
		Content string `json:"content,omitempty"`
	}{
		Alias:   (*Alias)(&m),
		Content: "",
	})
}

// OpenAIMessageContent 定义消息内容结构
type OpenAIMessageContent struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *OpenAIImageURL   `json:"image_url,omitempty"`
}

// OpenAIImageURL 定义图片URL结构
type OpenAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// OpenAIResponse 定义OpenAI API响应结构
type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// OpenAIChoice 定义OpenAI响应选择结构
type OpenAIChoice struct {
	Index   int `json:"index"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

// OpenAIStreamResponse 定义OpenAI流式响应的单个消息
type OpenAIStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		Delta        Delta  `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Delta 表示部分响应内容
type Delta struct {
	Content   string `json:"content"`
	// reasoning_content：部分 OpenAI 兼容后端会把推理输出放在该字段
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Role      string `json:"role,omitempty"`
}

// NewOpenAIModel 创建新的OpenAI模型
func NewOpenAIModel(cfg config.ModelConfig, proxies []config.ProxyConfig) (*OpenAIModel, error) {
	// 创建默认客户端
	defaultClient := &http.Client{
		Timeout: 600 * time.Second,
	}

	// 创建代理客户端映射
	proxyClients := make(map[string]*http.Client)

	// 为每个代理创建对应的HTTP客户端
	for _, proxy := range proxies {
		// 解析代理URL
		parsedURL, err := url.Parse(proxy.URL)
		if err != nil {
			log.Printf("解析代理URL失败 (%s): %v", proxy.Name, err)
			continue
		}

		// 创建带有代理的Transport
		transport := &http.Transport{
			Proxy: http.ProxyURL(parsedURL),
		}

		// 创建客户端并存储
		proxyClients[proxy.Name] = &http.Client{
			Transport: transport,
			Timeout:   600 * time.Second,
		}
	}

	m := &OpenAIModel{
		BaseModel: BaseModel{
			config: cfg,
		},
		defaultClient: defaultClient,
		proxyClients:  proxyClients,
	}
	
	// 加载tokenizer
	m.LoadTokenizer()
	
	return m, nil
}

// GenerateResponse 生成响应，发送实际的API请求
func (m *OpenAIModel) GenerateResponse(ctx context.Context, systemMessage, userMessage string, stream bool) (*LLMResponse, error) {
	// 选择合适的HTTP客户端
	client := m.defaultClient

	// 如果模型配置了代理，并且代理客户端存在，则使用代理客户端
	if m.config.ProxyName != "" {
		if proxyClient, ok := m.proxyClients[m.config.ProxyName]; ok {
			client = proxyClient
			log.Printf("使用代理: %s", m.config.ProxyName)
		} else {
			log.Printf("未找到配置的代理: %s，使用默认客户端", m.config.ProxyName)
		}
	}

	// 构建请求
	var streamOptions *StreamOptions
	if stream {
		// 流式模式下请求返回 usage 统计
		streamOptions = &StreamOptions{IncludeUsage: true}
	}
	
	reqBody := OpenAIRequest{
		Model: m.config.Params["model"].(string),
		Messages: []OpenAIMessage{
			{
				Role: "system",
				Text: systemMessage,
				// Content: []OpenAIMessageContent{
				// 	{
				// 		Type: "text",
				// 		Text: systemMessage,
				// 	},
				// },
			},
			{
				Role: "user",
				Text: userMessage,
				// Content: []OpenAIMessageContent{
				// 	{
				// 		Type: "text",
				// 		Text: userMessage,
				// 	},
				// },
			},
		},
		Temperature:   getFloat64(m.config.Params["temperature"]),
		MaxTokens:     getInt(m.config.Params["max_tokens"]),
		Stream:        stream,
		StreamOptions: streamOptions,
	}

	// 序列化请求体
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		m.config.BaseURL+"/chat/completions",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.config.APIKey))

	// 发送请求
	startTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API请求失败: 状态码=%d, 响应=%s", resp.StatusCode, string(body))
	}

	// 初始化返回结果
	result := &LLMResponse{
		Content:      "",
		InputTokens:  0,
		OutputTokens: 0,
	}

	// 非流式响应处理
	if !stream {
		// 读取响应体
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("读取响应体失败: %w", err)
		}

		// 打印请求延迟（可选，用于调试）

		// 解析响应
		var openAIResp OpenAIResponse
		if err := json.Unmarshal(body, &openAIResp); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}

		// 构建返回结果
		result.InputTokens = openAIResp.Usage.PromptTokens
		result.OutputTokens = openAIResp.Usage.CompletionTokens
		// 非流式模式不设置首token延迟，因为没有首token的概念
		// result.TimeToFirstToken = 0 (默认值)

		// 提取内容
		if len(openAIResp.Choices) > 0 {
			content := openAIResp.Choices[0].Message.Content
			result.Content = content
		}
	} else {
		// 流式响应处理
		var fullContent string
		var tokenText string
		var tokenCount int
		var firstTokenReceived bool
		var firstTokenTime time.Duration
		var tokenStartTime time.Time
		var totalInputTokens int
		var totalOutputTokens int

		// 创建一个新的reader
		reader := bufio.NewReader(resp.Body)

	LOOP:
		for {
			// 检查是否需要取消
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				// 读取一行数据，格式是 data: {...}
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break LOOP
					}
					return nil, fmt.Errorf("读取流式响应失败: %w", err)
				}

				// 去除前缀和空行
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if line == "[DONE]" {
					break LOOP
				}

				// 解析JSON，前缀通常是 "data: "
				if strings.HasPrefix(line, "data: ") {
					dataJSON := strings.TrimPrefix(line, "data: ")
					if dataJSON == "" {
						continue
					}

					if dataJSON == "[DONE]" {
						break LOOP
					}

					var streamResp OpenAIStreamResponse
					if err := json.Unmarshal([]byte(dataJSON), &streamResp); err != nil {
						log.Printf("解析流响应块失败: %v, 数据: %s", err, dataJSON)
						continue
					}


					// token统计（通常在最后一个消息中）
					if streamResp.Usage != nil {
						tokenCount = streamResp.Usage.TotalTokens
						totalInputTokens = streamResp.Usage.PromptTokens
						totalOutputTokens = streamResp.Usage.CompletionTokens
					}

					// 累加内容
					if len(streamResp.Choices) > 0 {
						delta := streamResp.Choices[0].Delta

						// “首个有效 token”判定：必须有实际可见文本（避免 role-only chunk 提前触发 TTFT）
						tokenPiece := delta.Content
						if tokenPiece == "" {
							if delta.ReasoningContent != "" {
								tokenPiece = delta.ReasoningContent
							} else if delta.Reasoning != "" {
								tokenPiece = delta.Reasoning
							}
						}

						if tokenPiece != "" {
							if !firstTokenReceived {
								firstTokenReceived = true
								firstTokenTime = time.Since(startTime)
								tokenStartTime = time.Now()
							}

							// token 统计用：包含 reasoning/content（用于 usage 缺失时的 tokenizer fallback）
							tokenText += tokenPiece

							// UI/内容展示用：仅保留 content，避免把 reasoning 混进最终展示
							if delta.Content != "" {
								fullContent += delta.Content
							}

							// warmup 场景：拿到首个有效 token 就退出
							if ShouldStopAfterFirstToken(ctx) {
								break LOOP
							}
						}
					}

				}
			}

		}

		// 流式响应结束，计算总延迟时间
		// 打印请求延迟（可选，用于调试）

		// 设置流式响应结果
		result.Content = fullContent
		result.InputTokens = totalInputTokens
		result.OutputTokens = totalOutputTokens

		// 如果没有获取到token统计，使用tokenizer精确计算
		inputText := fmt.Sprintf("%s %s", systemMessage, userMessage)
		if totalInputTokens == 0 {
			result.InputTokens = m.CountTokens(inputText)
		}
		if totalOutputTokens == 0 {
			// tokenText 包含 reasoning/content，更接近真实输出
			if tokenText == "" {
				tokenText = fullContent
			}
			result.OutputTokens = m.CountTokens(tokenText)
		}

		// 设置流式特定指标
		if firstTokenReceived {
			result.TimeToFirstToken = firstTokenTime
			if tokenCount > 0 {
				tokensPerSecond := float64(tokenCount) / time.Since(tokenStartTime).Seconds()
				result.TokensPerSecond = tokensPerSecond
			}
		}


	}

	return result, nil
}

// GenerateVisionResponse 生成多模态响应
func (m *OpenAIModel) GenerateVisionResponse(ctx context.Context, systemMessage, userMessage, imagePath string, stream bool) (*LLMResponse, error) {
	// 读取并编码图片
	imageData, err := m.encodeImageToBase64(imagePath)
	if err != nil {
		return nil, fmt.Errorf("处理图片失败: %w", err)
	}


	// 构建多模态消息
	messages := []OpenAIMessage{
		{
			Role: "system",
			Text: systemMessage,
		},
		{
			Role: "user",
			Content: []OpenAIMessageContent{
				{
					Type: "text",
					Text: userMessage,
				},
				{
					Type: "image_url",
					ImageURL: &OpenAIImageURL{
						URL: fmt.Sprintf("data:image/jpeg;base64,%s", imageData),
						Detail: "high",
					},
				},
			},
		},
	}

	// 直接构建带图片的API请求
	return m.generateVisionRequest(ctx, messages, stream)
}

// SupportsVision 返回OpenAI模型支持多模态
func (m *OpenAIModel) SupportsVision() bool {
	return true
}

// encodeImageToBase64 将图片文件编码为Base64
func (m *OpenAIModel) encodeImageToBase64(imagePath string) (string, error) {
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("读取图片文件失败: %w", err)
	}
	
	return base64.StdEncoding.EncodeToString(imageBytes), nil
}

// generateVisionRequest 构建并发送多模态API请求
func (m *OpenAIModel) generateVisionRequest(ctx context.Context, messages []OpenAIMessage, stream bool) (*LLMResponse, error) {
	// 选择合适的HTTP客户端
	var client *http.Client
	if m.config.ProxyName != "" {
		if proxyClient, ok := m.proxyClients[m.config.ProxyName]; ok {
			client = proxyClient
		} else {
			client = m.defaultClient
		}
	} else {
		client = m.defaultClient
	}

	// 构建请求体
	requestBody := map[string]interface{}{
		"model":      m.config.Params["model"],
		"messages":   messages,
		"stream":     stream,
		"max_tokens": m.config.Params["max_tokens"],
	}
	// 流式模式下请求返回 usage 统计
	if stream {
		requestBody["stream_options"] = map[string]interface{}{
			"include_usage": true,
		}
	}

	// 添加其他参数
	if temp, ok := m.config.Params["temperature"]; ok {
		requestBody["temperature"] = temp
	}
	if topP, ok := m.config.Params["top_p"]; ok {
		requestBody["top_p"] = topP
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}


	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", m.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.config.APIKey)

	// 记录开始时间
	startTime := time.Now()

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API请求失败: 状态码=%d, 响应=%s", resp.StatusCode, string(body))
	}

	// 使用现有的流式响应处理逻辑，并为 tokenizer fallback 提供可用的文本上下文
	systemText, userText := extractSystemAndUserText(messages)
	return m.processStreamResponse(ctx, resp, startTime, stream, systemText, userText)
}

// extractSystemAndUserText 从 messages 中提取系统/用户文本，用于 token 统计的兜底
func extractSystemAndUserText(messages []OpenAIMessage) (string, string) {
	var systemText string
	var userText string
	for _, msg := range messages {
		if msg.Role == "system" && systemText == "" {
			systemText = msg.Text
		}
		if msg.Role == "user" && userText == "" {
			if msg.Text != "" {
				userText = msg.Text
				continue
			}
			// 多模态：优先拼接 text 类型内容，忽略 image_url
			if len(msg.Content) > 0 {
				var b strings.Builder
				for _, c := range msg.Content {
					if c.Type == "text" && c.Text != "" {
						if b.Len() > 0 {
							b.WriteByte(' ')
						}
						b.WriteString(c.Text)
					}
				}
				userText = strings.TrimSpace(b.String())
			}
		}
	}
	return systemText, userText
}

// processStreamResponse 处理流式响应（从现有GenerateResponse方法中提取）
func (m *OpenAIModel) processStreamResponse(ctx context.Context, resp *http.Response, startTime time.Time, stream bool, systemMessage, userMessage string) (*LLMResponse, error) {
	// 初始化返回结果
	result := &LLMResponse{
		Content:      "",
		InputTokens:  0,
		OutputTokens: 0,
	}

	// 非流式响应处理
	if !stream {
		// 读取响应体
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("读取响应体失败: %w", err)
		}


		// 解析响应
		var openAIResp OpenAIResponse
		if err := json.Unmarshal(body, &openAIResp); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}

		// 构建返回结果
		result.InputTokens = openAIResp.Usage.PromptTokens
		result.OutputTokens = openAIResp.Usage.CompletionTokens

		// 提取内容
		if len(openAIResp.Choices) > 0 {
			content := openAIResp.Choices[0].Message.Content
			result.Content = content
		}
	} else {
		// 流式响应处理
		var fullContent string
		var tokenText string
		var tokenCount int
		var firstTokenReceived bool
		var firstTokenTime time.Duration
		var tokenStartTime time.Time
		var totalInputTokens int
		var totalOutputTokens int

		// 创建一个reader
		reader := bufio.NewReader(resp.Body)

	LOOP:
		for {
			// 检查是否需要取消
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				// 读取一行数据，格式是 data: {...}
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break LOOP
					}
					return nil, fmt.Errorf("读取流式响应失败: %w", err)
				}

				// 去除前缀和空行
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if line == "[DONE]" {
					break LOOP
				}

				// 解析JSON，前缀通常是 "data: "
				if strings.HasPrefix(line, "data: ") {
					dataJSON := strings.TrimPrefix(line, "data: ")
					if dataJSON == "" {
						continue
					}

					if dataJSON == "[DONE]" {
						break LOOP
					}

					var streamResp OpenAIStreamResponse
					if err := json.Unmarshal([]byte(dataJSON), &streamResp); err != nil {
						log.Printf("解析流响应块失败: %v, 数据: %s", err, dataJSON)
						continue
					}


					// token统计（通常在最后一个消息中）
					if streamResp.Usage != nil {
						tokenCount = streamResp.Usage.TotalTokens
						totalInputTokens = streamResp.Usage.PromptTokens
						totalOutputTokens = streamResp.Usage.CompletionTokens
					}

					// 累加内容
					if len(streamResp.Choices) > 0 {
						delta := streamResp.Choices[0].Delta

						// “首个有效 token”判定：必须有实际可见文本（避免 role-only chunk 提前触发 TTFT）
						tokenPiece := delta.Content
						if tokenPiece == "" {
							if delta.ReasoningContent != "" {
								tokenPiece = delta.ReasoningContent
							} else if delta.Reasoning != "" {
								tokenPiece = delta.Reasoning
							}
						}

						if tokenPiece != "" {
							if !firstTokenReceived {
								firstTokenReceived = true
								firstTokenTime = time.Since(startTime)
								tokenStartTime = time.Now()
							}

							tokenText += tokenPiece

							if delta.Content != "" {
								fullContent += delta.Content
							}

							if ShouldStopAfterFirstToken(ctx) {
								break LOOP
							}
						}
					}

				}
			}

		}

		// 流式响应结束，计算总延迟时间

		// 设置流式响应结果
		result.Content = fullContent
		result.InputTokens = totalInputTokens
		result.OutputTokens = totalOutputTokens

		// 如果没有获取到token统计，使用tokenizer精确计算
		inputText := fmt.Sprintf("%s %s", systemMessage, userMessage)
		if totalInputTokens == 0 {
			result.InputTokens = m.CountTokens(inputText)
		}
		if totalOutputTokens == 0 {
			if tokenText == "" {
				tokenText = fullContent
			}
			result.OutputTokens = m.CountTokens(tokenText)
		}

		// 设置流式特定指标
		if firstTokenReceived {
			result.TimeToFirstToken = firstTokenTime
			if tokenCount > 0 {
				tokensPerSecond := float64(tokenCount) / time.Since(tokenStartTime).Seconds()
				result.TokensPerSecond = tokensPerSecond
			}
		}

	}

	return result, nil
}
