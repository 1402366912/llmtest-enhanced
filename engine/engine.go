package engine

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/briandowns/spinner"
	"github.com/lemonlinger/llm-test/config"
	"github.com/lemonlinger/llm-test/model"
)

// 测试结果结构体
type TestResult struct {
	ModelName          string
	ConcurrencyLevel   int // 添加并发度字段
	// 上下文目标长度（Token），用于分组与报告
	ContextTargetTokens int
	TotalRequests      int
	SuccessRequests    int
	FailedRequests     int
	TotalDuration      time.Duration
	AvgLatency         time.Duration
	InputTokens        int64
	OutputTokens       int64
	TotalTokens        int64
	AvgInputTokens     float64
	AvgOutputTokens    float64
	AvgTotalTokens     float64
	RequestsPerSec     float64
	TokensPerSec       float64
	// 阶段指标（平均值）
	PrefillTokensPerSecAvg float64 // = 输入tokens / TTFT
	DecodeTokensPerSecAvg  float64 // = 输出tokens / (总时长 - TTFT)
	// 实际有效并发数（通过时间重叠分析计算）
	EffectiveConcurrency float64 `json:"effective_concurrency"`
	Errors             []string
	LatencyPercentiles map[int]time.Duration // 存储各个百分位的延迟
	AllLatencies       []time.Duration       // 所有请求的延迟记录
	// 首token延迟相关指标
	FirstTokenLatencies []time.Duration // 所有请求的首token延迟记录
	AvgFirstTokenLatency time.Duration  // 平均首token延迟
	FirstTokenPercentiles map[int]time.Duration // 首token延迟百分位
}

// 测试引擎结构体
type TestEngine struct {
	config   config.TestConfig
	models   []model.LLMModel
	prompt   config.PromptConfig
	results  map[string]*TestResult
	spinner  *spinner.Spinner
	proxies  map[string]string // 代理名称到URL的映射
	dataset  []string          // 数据集（多个提示词）
	useDataset bool            // 是否使用数据集
}

// 创建新的测试引擎
func NewTestEngine(testConfig config.TestConfig, models []model.LLMModel, prompt config.PromptConfig, proxies []config.ProxyConfig) *TestEngine {
	// 创建代理映射
	proxyMap := make(map[string]string)
	for _, proxy := range proxies {
		proxyMap[proxy.Name] = proxy.URL
	}

	engine := &TestEngine{
		config:     testConfig,
		models:     models,
		prompt:     prompt,
		results:    make(map[string]*TestResult),
		proxies:    proxyMap,
		dataset:    []string{},
		useDataset: false,
	}

	// 加载数据集（如果配置了）
	if prompt.DatasetPath != "" {
		dataset, err := loadDataset(prompt.DatasetPath)
		if err != nil {
			log.Printf("警告: 加载数据集失败 (%s): %v，将使用固定提示词", prompt.DatasetPath, err)
		} else if len(dataset) > 0 {
			engine.dataset = dataset
			engine.useDataset = true
			log.Printf("✓ 已加载数据集: %s (%d 条提示词)", prompt.DatasetPath, len(dataset))
		}
	}

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	return engine
}

// loadDataset 从文件加载数据集（每行一个提示词）
func loadDataset(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var prompts []string
	scanner := bufio.NewScanner(file)
	// 增大缓冲区以支持长提示词
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") { // 跳过空行和注释
			prompts = append(prompts, line)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	return prompts, nil
}

// getRandomPrompt 从数据集中随机获取一个提示词
func (e *TestEngine) getRandomPrompt() string {
	if e.useDataset && len(e.dataset) > 0 {
		return e.dataset[rand.Intn(len(e.dataset))]
	}
	return e.prompt.UserMessage
}

// 运行测试
func (e *TestEngine) Run() (map[string]*TestResult, error) {
	// 使用复合键（模型名称+并发度）来存储结果
	results := make(map[string]*TestResult)

	for _, mdl := range e.models {
		modelName := mdl.GetName()
		fmt.Printf("正在测试模型: %s\n", modelName)

		// 上下文长度梯度
		contextLevels := e.config.ContextTokenLevels
		if len(contextLevels) == 0 {
			contextLevels = []int{0} // 0 表示无额外上下文
		} else {
			fmt.Printf("  使用上下文长度梯度: %v\n", contextLevels)
		}

		// 设置并发度：优先使用模型自身的并发度配置，如果没有则使用全局配置
		var concurrencyLevels []int
		modelConcurrencyLevels := mdl.GetConcurrencyLevels()

		if len(modelConcurrencyLevels) > 0 {
			// 使用模型特定的并发度配置
			concurrencyLevels = modelConcurrencyLevels
			fmt.Printf("  使用模型特定的并发度配置: %v\n", concurrencyLevels)
		} else if len(e.config.ConcurrencyLevels) > 0 {
			// 使用全局并发度级别列表
			concurrencyLevels = e.config.ConcurrencyLevels
			fmt.Printf("  使用全局并发度配置: %v\n", concurrencyLevels)
		} else {
			// 使用基础并发度
			concurrencyLevels = []int{e.config.Concurrency}
			fmt.Printf("  使用基础并发度: %d\n", e.config.Concurrency)
		}

		// 对每个并发级别、每个上下文长度运行测试（先固定并发，遍历上下文）
		for _, concurrency := range concurrencyLevels {
		for _, ctxLen := range contextLevels {
			// 生成上下文填充文本（近似，根据目标token数生成一定长度的文本）
			contextText := e.generateContextText(ctxLen)

				// 为每个并发度与上下文长度创建一个新的结果对象
				resultKey := fmt.Sprintf("%s-%d-%dctx", modelName, concurrency, ctxLen)
				result := &TestResult{
					ModelName:           modelName,
					ConcurrencyLevel:    concurrency,
					ContextTargetTokens: ctxLen,
					Errors:              make([]string, 0),
				}
				results[resultKey] = result

				err := e.runTestWithConcurrency(mdl, concurrency, ctxLen, contextText, result)
				if err != nil {
					return nil, fmt.Errorf("测试模型 %s 失败: %w", modelName, err)
				}
			}
		}
	}

	// 更新e.results以保持兼容性
	e.results = results

	return results, nil
}

// 以指定并发度与上下文长度运行测试
func (e *TestEngine) runTestWithConcurrency(mdl model.LLMModel, concurrency int, contextTargetTokens int, contextText string, result *TestResult) error {
	// 获取模型名称
	modelName := mdl.GetName()

	fmt.Printf("  并发度: %d, 上下文目标Tokens: %d\n", concurrency, contextTargetTokens)

	// 初始化计数器
	var successCount int64
	var failedCount int64
	var totalLatency int64
	var inputTokens int64
	var outputTokens int64
	var sumPrefillTPS float64
	var sumDecodeTPS float64
	var numPrefillSamples int64
	var numDecodeSamples int64

	// 确定是否使用流式输出：优先使用模型特定设置，如果未设置则使用全局设置
	useStream := e.prompt.Stream
	if modelStream := mdl.GetStreamSetting(); modelStream != nil {
		useStream = *modelStream
		fmt.Printf("  使用模型特定的流式设置: %v\n", useStream)
	} else {
		fmt.Printf("  使用全局流式设置: %v\n", useStream)
	}

	if e.config.ShowProgress {
		e.spinner = spinner.New(spinner.CharSets[9], 100*time.Millisecond)
		e.spinner.Prefix = "  正在测试 "
		e.spinner.Start()
		defer e.spinner.Stop()
	}

	// 创建工作通道和等待组
	jobs := make(chan struct{}, concurrency*2)
	var wg sync.WaitGroup

	// 创建延迟数据切片和互斥锁
	var latencies []time.Duration
	var firstTokenLatencies []time.Duration
	var latenciesMutex sync.Mutex
	// 收集错误的互斥锁（避免并发 append 产生数据竞争）
	var errorsMutex sync.Mutex
	
	// 用于打印第一次回复的标志
	var firstResponsePrinted int32

	// 创建信号量控制并发
	sem := make(chan struct{}, concurrency)

	// 如果有预热时间，先进行预热
	if e.config.WarmupDuration > 0 {
		// 预热逻辑...
		time.Sleep(e.config.WarmupDuration.Duration())
	}

	// 记录开始时间
	startTime := time.Now()

	// 启动工作协程
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for range jobs {
				sem <- struct{}{}

				// 执行单个请求
				// 如果 RequestTimeout 为 0，则不设置超时
				var ctx context.Context
				var cancel context.CancelFunc
				if e.config.RequestTimeout > 0 {
					ctx, cancel = context.WithTimeout(context.Background(), e.config.RequestTimeout.Duration())
				} else {
					ctx, cancel = context.WithCancel(context.Background())
				}

				start := time.Now()
				var resp *model.LLMResponse
				var err error

				// 获取提示词（从数据集随机选取或使用固定提示词）
				currentPrompt := e.getRandomPrompt()
				
				// 组装带上下文的用户消息
				userMsg := currentPrompt
				if contextTargetTokens > 0 && contextText != "" {
					userMsg = fmt.Sprintf("[CONTEXT LEN=%d]\n%s\n\n任务：%s", contextTargetTokens, contextText, currentPrompt)
				}
				
				// 检查是否是多模态模型且有图片输入
				if mdl.SupportsVision() && (e.prompt.ImagePath != "" || e.prompt.ImageURL != "") {
					imagePath := e.prompt.ImagePath
					if imagePath == "" && e.prompt.ImageURL != "" {
						// TODO: 下载图片URL到本地文件
						imagePath = e.prompt.ImageURL
					}
					resp, err = mdl.GenerateVisionResponse(ctx, e.prompt.SystemMessage, userMsg, imagePath, useStream)
				} else {
					resp, err = mdl.GenerateResponse(ctx, e.prompt.SystemMessage, userMsg, useStream)
				}
				latency := time.Since(start)

				// 记录延迟数据
				latenciesMutex.Lock()
				latencies = append(latencies, latency)
				// 记录首token延迟（仅在成功响应且有首token延迟数据时）
				if err == nil && resp.TimeToFirstToken > 0 {
					firstTokenLatencies = append(firstTokenLatencies, resp.TimeToFirstToken)
				}
				latenciesMutex.Unlock()

				if err != nil {
					log.Printf("测试模型 %s 失败: %v", modelName, err)
					atomic.AddInt64(&failedCount, 1)
					errorsMutex.Lock()
					result.Errors = append(result.Errors, err.Error())
					errorsMutex.Unlock()
				} else {
					atomic.AddInt64(&successCount, 1)
					atomic.AddInt64(&totalLatency, int64(latency))
					atomic.AddInt64(&inputTokens, int64(resp.InputTokens))
					atomic.AddInt64(&outputTokens, int64(resp.OutputTokens))

					// 阶段TPS（仅在启用且有TTFT的情况下计算）
					if e.config.EnablePrefillMetrics && resp.TimeToFirstToken > 0 {
						prefillSec := resp.TimeToFirstToken.Seconds()
						if prefillSec > 0 && resp.InputTokens > 0 {
							atomic.AddInt64(&numPrefillSamples, 1)
							// 用互斥保护浮点累加（这里直接加，因为每次只有一个G协程持有sumPrefillTPS没有原子性；用互斥更安全）
							latenciesMutex.Lock()
							sumPrefillTPS += float64(resp.InputTokens) / prefillSec
							latenciesMutex.Unlock()
						}
						decodeSec := latency.Seconds() - prefillSec
						if decodeSec > 0 && resp.OutputTokens > 0 {
							atomic.AddInt64(&numDecodeSamples, 1)
							latenciesMutex.Lock()
							sumDecodeTPS += float64(resp.OutputTokens) / decodeSec
							latenciesMutex.Unlock()
						}
					}
					
					// 打印第一次成功的回复内容
					if atomic.CompareAndSwapInt32(&firstResponsePrinted, 0, 1) {
						fmt.Printf("\n=== %s 第一次回复示例 ===\n", modelName)
						fmt.Printf("回复内容: %s\n", resp.Content)
						fmt.Printf("输入Token: %d, 输出Token: %d, 总Token: %d\n", 
							resp.InputTokens, resp.OutputTokens, resp.InputTokens+resp.OutputTokens)
						if resp.TimeToFirstToken > 0 {
							fmt.Printf("首Token延迟: %v\n", resp.TimeToFirstToken)
						}
						fmt.Printf("总延迟: %v\n", latency)
						fmt.Printf("=============================\n\n")
					}
				}

				cancel()
				<-sem
			}
		}()
	}

	// 发送工作
	// 使用 stopSending 来控制是否继续发送新请求
	stopSending := make(chan struct{})
	go func() {
		time.Sleep(e.config.Duration.Duration())
		close(stopSending)
	}()

	sentCount := 0

loop:
	for {
		select {
		case <-stopSending:
			// 测试时间到，停止发送新请求
			fmt.Printf("  测试时间到，停止发送新请求（已发送 %d 个）\n", sentCount)
			break loop
		case jobs <- struct{}{}:
			sentCount++
		}
	}

	// 关闭 jobs channel，不再发送新任务
	// 但 workers 会继续处理 channel 里剩余的任务
	close(jobs)
	
	// 等待所有已发出的请求完成
	fmt.Printf("  等待正在执行的请求完成...\n")
	wg.Wait()

	// 计算总持续时间（包括等待最后一批请求完成的时间）
	totalDuration := time.Since(startTime)
	fmt.Printf("  所有请求已完成，总耗时: %v\n", totalDuration)

	// 使用实际完成的请求数来更新结果
	actualTotal := int(successCount) + int(failedCount)
	result.TotalRequests = actualTotal
	result.SuccessRequests = int(successCount)
	result.FailedRequests = int(failedCount)
	result.TotalDuration = totalDuration
	result.ContextTargetTokens = contextTargetTokens

	if successCount > 0 {
		avgLatency := time.Duration(totalLatency / successCount)
		result.AvgLatency = avgLatency

		// 直接赋值，避免潜在的重复累加问题
		result.InputTokens = inputTokens
		result.OutputTokens = outputTokens
		result.TotalTokens = inputTokens + outputTokens

		result.AvgInputTokens = float64(result.InputTokens) / float64(result.SuccessRequests)
		result.AvgOutputTokens = float64(result.OutputTokens) / float64(result.SuccessRequests)
		result.AvgTotalTokens = float64(result.TotalTokens) / float64(result.SuccessRequests)

		result.RequestsPerSec = float64(result.SuccessRequests) / totalDuration.Seconds()
		result.TokensPerSec = float64(result.TotalTokens) / totalDuration.Seconds()

		// 阶段TPS平均值
		if numPrefillSamples > 0 {
			result.PrefillTokensPerSecAvg = sumPrefillTPS / float64(numPrefillSamples)
		}
		// 每请求实际生成速度 = 总TPS / 并发数
		// 这反映了在并发场景下，每个用户实际体验到的生成速度
		if concurrency > 0 {
			result.DecodeTokensPerSecAvg = result.TokensPerSec / float64(concurrency)
		}
	}

	// 存储所有延迟数据
	result.AllLatencies = latencies
	result.FirstTokenLatencies = firstTokenLatencies

	// 计算延迟百分位
	if len(latencies) > 0 && len(e.config.LatencyPercentiles) > 0 {
		result.LatencyPercentiles = make(map[int]time.Duration)
		for _, p := range e.config.LatencyPercentiles {
			result.LatencyPercentiles[p] = calculatePercentile(latencies, p)
		}
	}

	// 计算首token延迟的平均值和百分位
	if len(firstTokenLatencies) > 0 {
		// 计算平均首token延迟
		var totalFirstTokenLatency int64
		for _, ftl := range firstTokenLatencies {
			totalFirstTokenLatency += int64(ftl)
		}
		result.AvgFirstTokenLatency = time.Duration(totalFirstTokenLatency / int64(len(firstTokenLatencies)))

		// 计算首token延迟百分位
		if len(e.config.LatencyPercentiles) > 0 {
			result.FirstTokenPercentiles = make(map[int]time.Duration)
			for _, p := range e.config.LatencyPercentiles {
				result.FirstTokenPercentiles[p] = calculatePercentile(firstTokenLatencies, p)
			}
		}
	}

	return nil
}

// 计算百分位数
func calculatePercentile(latencies []time.Duration, percentile int) time.Duration {
	// 创建副本并排序
	sortedLatencies := make([]time.Duration, len(latencies))
	copy(sortedLatencies, latencies)
	sort.Slice(sortedLatencies, func(i, j int) bool {
		return sortedLatencies[i] < sortedLatencies[j]
	})

	// 计算百分位索引
	index := int(float64(len(sortedLatencies)-1) * float64(percentile) / 100.0)
	return sortedLatencies[index]
}

// 生成指定目标token数量的上下文填充文本（近似）
// 简化策略：按目标token数产生接近数量的汉字/短句，真实token以API usage为准。
func (e *TestEngine) generateContextText(targetTokens int) string {
	if targetTokens <= 0 {
		return ""
	}
	base := e.prompt.ContextBase
	if base == "" {
		base = "这是用于性能测试的上下文填充句子，用于模拟较长的历史与知识背景。请忽略其语义，仅用于长度测试。"
	}
	// 经验：中文大致1字≈1 token（不同模型略有偏差），这里按1:1近似生成
	// 采用重复句子避免过多重复同字影响分词，加入少量变体标记
	builder := make([]rune, 0, targetTokens+64)
	suffixes := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	sr := []rune(base)
	idx := 0
	for len(builder) < targetTokens {
		// 追加一句
		builder = append(builder, sr...)
		// 添加一个区隔符与变体字符，降低过长重复片段的合并影响
		builder = append(builder, '（', '片', '段', suffixes[idx%len(suffixes)], '）', '。')
		idx++
	}
	return string(builder)
}
