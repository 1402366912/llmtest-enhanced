package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemonlinger/llm-test/config"
	"github.com/lemonlinger/llm-test/model"
)

// ProgressInfo 进度信息
type ProgressInfo struct {
	ModelName       string
	Concurrency     int
	ContextTokens   int
	CurrentRequests int
	TotalRequests   int
	SuccessRequests int
	FailedRequests  int
	AvgTTFTMs       float64 // 平均首Token延迟(毫秒)
	PrefillTPS      float64 // 预填充速度 (输入tokens/首token延迟)
	RPS             float64
	TPS             float64 // 总体TPS (输出tokens/总时间)
	DecodeTPSAvg    float64 // 平均生成速度 (每请求的 输出tokens/生成时间)
}

// ProgressCallback 进度回调函数类型
type ProgressCallback func(*ProgressInfo)

// requestTimingInfo 记录每个请求的时间信息，用于计算实际有效并发数
type requestTimingInfo struct {
	startTime    time.Time     // 请求开始时间
	ttft         time.Duration // 首token延迟（prefill时间）
	endTime      time.Time     // 请求结束时间
	outputTokens int           // 输出token数
}

// timingEvent 时间事件，用于扫描线算法
type timingEvent struct {
	timestamp time.Time
	delta     int // +1 表示 decode 开始，-1 表示 decode 结束
}

// calculateEffectiveConcurrency 使用扫描线算法计算实际有效并发数
// 通过分析所有请求的 decode 时间段重叠情况来确定
func calculateEffectiveConcurrency(timings []requestTimingInfo) float64 {
	if len(timings) == 0 {
		return 0
	}

	// 过滤掉没有有效 TTFT 的请求（非流式模式）
	var validTimings []requestTimingInfo
	for _, t := range timings {
		if t.ttft > 0 {
			validTimings = append(validTimings, t)
		}
	}

	if len(validTimings) == 0 {
		// 没有有效的流式数据，返回 0 表示无法计算
		return 0
	}

	// 创建事件列表
	events := make([]timingEvent, 0, len(validTimings)*2)
	for _, t := range validTimings {
		decodeStart := t.startTime.Add(t.ttft) // decode 开始时间 = 请求开始 + TTFT
		decodeEnd := t.endTime                  // decode 结束时间 = 请求结束

		// 确保 decode 时间段有效
		if decodeEnd.After(decodeStart) {
			events = append(events, timingEvent{timestamp: decodeStart, delta: 1})
			events = append(events, timingEvent{timestamp: decodeEnd, delta: -1})
		}
	}

	if len(events) < 2 {
		return 1
	}

	// 按时间排序（相同时间时，结束事件在开始事件之前，避免瞬间并发数偏高）
	sort.Slice(events, func(i, j int) bool {
		if events[i].timestamp.Equal(events[j].timestamp) {
			return events[i].delta < events[j].delta // -1 在 +1 之前
		}
		return events[i].timestamp.Before(events[j].timestamp)
	})

	// 扫描线算法：计算时间加权平均并发数
	var currentConcurrency int
	var weightedSum float64
	var totalDuration float64

	for i := 0; i < len(events)-1; i++ {
		currentConcurrency += events[i].delta
		
		// 计算当前时间段的持续时间
		duration := events[i+1].timestamp.Sub(events[i].timestamp).Seconds()
		if duration > 0 && currentConcurrency > 0 {
			weightedSum += float64(currentConcurrency) * duration
			totalDuration += duration
		}
	}

	if totalDuration <= 0 {
		return float64(len(validTimings))
	}

	return weightedSum / totalDuration
}

// TestEngineWithProgress 带进度回调的测试引擎
type TestEngineWithProgress struct {
	*TestEngine
	progressCallback ProgressCallback
	ctx              context.Context
}

// NewTestEngineWithProgress 创建带进度回调的测试引擎
func NewTestEngineWithProgress(testConfig config.TestConfig, models []model.LLMModel, prompt config.PromptConfig, proxies []config.ProxyConfig, callback ProgressCallback) *TestEngineWithProgress {
	base := NewTestEngine(testConfig, models, prompt, proxies)
	return &TestEngineWithProgress{
		TestEngine:       base,
		progressCallback: callback,
	}
}

// RunWithContext 带上下文的测试运行
func (e *TestEngineWithProgress) RunWithContext(ctx context.Context) (map[string]*TestResult, error) {
	e.ctx = ctx
	results := make(map[string]*TestResult)

	for _, mdl := range e.models {
		// 检查是否取消
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		modelName := mdl.GetName()
		log.Printf("正在测试模型: %s", modelName)

		// 确定是否使用流式输出（warmup与正式测试保持一致）
		useStream := e.prompt.Stream
		if modelStream := mdl.GetStreamSetting(); modelStream != nil {
			useStream = *modelStream
		}

		// 每模型一次 warmup：拿到首个有效 token 即退出（或超时退出）
		if e.config.WarmupRequests > 0 {
			warmupTimeout := e.config.WarmupTimeout.Duration()
			if warmupTimeout <= 0 {
				warmupTimeout = 10 * time.Second
			}
			for i := 0; i < e.config.WarmupRequests; i++ {
				select {
				case <-ctx.Done():
					return results, ctx.Err()
				default:
				}

				currentPrompt := e.getRandomPrompt()
				userMsg := currentPrompt

				warmupCtx, cancel := context.WithTimeout(ctx, warmupTimeout)
				if useStream {
					warmupCtx = model.WithStopAfterFirstToken(warmupCtx)
				}

				start := time.Now()
				var err error
				if mdl.SupportsVision() && (e.prompt.ImagePath != "" || e.prompt.ImageURL != "") {
					imagePath := e.prompt.ImagePath
					if imagePath == "" && e.prompt.ImageURL != "" {
						imagePath = e.prompt.ImageURL
					}
					_, err = mdl.GenerateVisionResponse(warmupCtx, e.prompt.SystemMessage, userMsg, imagePath, useStream)
				} else {
					_, err = mdl.GenerateResponse(warmupCtx, e.prompt.SystemMessage, userMsg, useStream)
				}
				cancel()

				if err != nil {
					log.Printf("warmup[%s] #%d 失败(耗时=%v): %v", modelName, i+1, time.Since(start), err)
				} else {
					log.Printf("warmup[%s] #%d 完成(耗时=%v)", modelName, i+1, time.Since(start))
				}
			}
		}

		// 上下文长度梯度
		contextLevels := e.config.ContextTokenLevels
		if len(contextLevels) == 0 {
			contextLevels = []int{0}
		}

		// 设置并发度
		var concurrencyLevels []int
		modelConcurrencyLevels := mdl.GetConcurrencyLevels()

		if len(modelConcurrencyLevels) > 0 {
			concurrencyLevels = modelConcurrencyLevels
		} else if len(e.config.ConcurrencyLevels) > 0 {
			concurrencyLevels = e.config.ConcurrencyLevels
		} else {
			concurrencyLevels = []int{e.config.Concurrency}
		}

		// 对每个并发级别、每个上下文长度运行测试（先固定并发，遍历上下文）
		for _, concurrency := range concurrencyLevels {
		for _, ctxLen := range contextLevels {
				// 检查是否取消
				select {
				case <-ctx.Done():
					return results, ctx.Err()
				default:
				}

				contextText := e.generateContextText(ctxLen)

				resultKey := fmt.Sprintf("%s-%d-%dctx", modelName, concurrency, ctxLen)
				result := &TestResult{
					ModelName:           modelName,
					ConcurrencyLevel:    concurrency,
					ContextTargetTokens: ctxLen,
					Errors:              make([]string, 0),
				}
				results[resultKey] = result

				err := e.runTestWithContextAndProgress(ctx, mdl, concurrency, ctxLen, contextText, result)
				if err != nil {
					if ctx.Err() != nil {
						return results, ctx.Err()
					}
					return nil, fmt.Errorf("测试模型 %s 失败: %w", modelName, err)
				}
			}
		}
	}

	e.results = results
	return results, nil
}

// runTestWithContextAndProgress 带上下文和进度回调的测试
// 支持两种模式：
// 1. 请求数模式（默认）：发送 并发数×3 的请求数，保持固定并发
// 2. 时间模式（StressTestMode=true）：在指定时间内持续发送请求（压力测试）
func (e *TestEngineWithProgress) runTestWithContextAndProgress(ctx context.Context, mdl model.LLMModel, concurrency int, contextTargetTokens int, contextText string, result *TestResult) error {
	modelName := mdl.GetName()
	
	// 判断是否为压力测试模式
	isStressTestMode := e.config.StressTestMode
	
	// 请求数模式：
	// - 并发数为1时，总请求数 = 5
	// - 其他情况，总请求数 = 并发数 × 3
	var totalRequestsTarget int
	if concurrency == 1 {
		totalRequestsTarget = 5
	} else {
		totalRequestsTarget = concurrency * 3
	}
	
	if isStressTestMode {
		log.Printf("  并发度: %d, 上下文目标Tokens: %d, 持续时间: %v (压力测试模式)", concurrency, contextTargetTokens, e.config.Duration)
	} else {
		log.Printf("  并发度: %d, 上下文目标Tokens: %d, 总请求数: %d (请求数模式)", concurrency, contextTargetTokens, totalRequestsTarget)
	}

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
	var requestsSent int64  // 已发送的请求数
	var totalTTFT int64
	var ttftCount int64
	var totalDecodeTime int64 // 累计生成时间（总延迟 - 首token延迟）

	// 请求数模式下的快照数据（达到目标时记录）
	var snapshotTaken int32 // 是否已记录快照
	var snapshotTime time.Time
	var snapshotOutputTokens int64
	var snapshotInputTokens int64
	var snapshotSuccessCount int64
	var snapshotFailedCount int64
	var snapshotTotalDecodeTime int64 // 快照时的累计生成时间
	var snapshotPrefillTPS float64
	var snapshotNumPrefillSamples int64
	var snapshotSumDecodeTPS float64    // 快照时的累计单请求decode TPS
	var snapshotNumDecodeSamples int64  // 快照时的decode样本数
	var snapshotTotalTTFT int64         // 快照时的累计首token延迟
	var snapshotTTFTCount int64         // 快照时的TTFT样本数
	var snapshotAllTimings []requestTimingInfo // 快照时的时间数据

	// 确定是否使用流式输出
	useStream := e.prompt.Stream
	if modelStream := mdl.GetStreamSetting(); modelStream != nil {
		useStream = *modelStream
	}

	var wg sync.WaitGroup

	// 延迟数据
	var latencies []time.Duration
	var firstTokenLatencies []time.Duration
	var allTimings []requestTimingInfo // 收集所有请求的时间信息，用于计算实际并发数
	var latenciesMutex sync.Mutex
	var errorsMutex sync.Mutex
	var firstResponsePrinted int32

	// 预热
	if e.config.WarmupDuration > 0 {
		time.Sleep(e.config.WarmupDuration.Duration())
	}

	// 记录开始时间
	startTime := time.Now()

	// 进度报告定时器
	progressTicker := time.NewTicker(500 * time.Millisecond)
	progressStop := make(chan struct{})

	// 启动进度报告协程
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		defer progressTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-progressStop:
				return
			case <-progressTicker.C:
				if e.progressCallback != nil {
					// 请求数模式下，达到目标后使用快照数据（冻结显示）
					if !isStressTestMode && atomic.LoadInt32(&snapshotTaken) == 1 {
						snapshotElapsed := snapshotTime.Sub(startTime).Seconds()
						var rps, tps, decodeTPSAvg, avgTTFTMs, prefillTPS float64
						// 使用有效生成时间计算 TPS
						effectiveDecodeSeconds := float64(snapshotTotalDecodeTime) / float64(time.Second) / float64(concurrency)
						if effectiveDecodeSeconds > 0 {
							tps = float64(snapshotOutputTokens) / effectiveDecodeSeconds
							decodeTPSAvg = tps / float64(concurrency)
						}
						if snapshotElapsed > 0 {
							rps = float64(snapshotSuccessCount) / snapshotElapsed
						}
						// 注意：TTFT 需要使用累计 TTFT，而不是累计 decodeTime
						if snapshotTTFTCount > 0 {
							avgTTFTMs = float64(snapshotTotalTTFT) / float64(snapshotTTFTCount) / 1e6
						}
						if snapshotNumPrefillSamples > 0 {
							prefillTPS = snapshotPrefillTPS / float64(snapshotNumPrefillSamples)
						}
						e.progressCallback(&ProgressInfo{
							ModelName:       modelName,
							Concurrency:     concurrency,
							ContextTokens:   contextTargetTokens,
							CurrentRequests: int(snapshotSuccessCount + snapshotFailedCount),
							TotalRequests:   totalRequestsTarget,
							SuccessRequests: int(snapshotSuccessCount),
							FailedRequests:  int(snapshotFailedCount),
							AvgTTFTMs:       avgTTFTMs,
							PrefillTPS:      prefillTPS,
							RPS:             rps,
							TPS:             tps,
							DecodeTPSAvg:    decodeTPSAvg,
						})
						continue
					}

					// 正常模式：实时计算
					elapsed := time.Since(startTime).Seconds()
					success := atomic.LoadInt64(&successCount)
					failed := atomic.LoadInt64(&failedCount)
					sent := atomic.LoadInt64(&requestsSent)
					outTokens := atomic.LoadInt64(&outputTokens)
					inTokens := atomic.LoadInt64(&inputTokens)
					ttft := atomic.LoadInt64(&totalTTFT)
					ttftCnt := atomic.LoadInt64(&ttftCount)
					prefillSamples := atomic.LoadInt64(&numPrefillSamples)

					var avgTTFTMs float64
					if ttftCnt > 0 {
						avgTTFTMs = float64(ttft) / float64(ttftCnt) / 1e6
					}

					// 计算 Prefill TPS：输入tokens / 首token延迟
					// 并发时取平均值
					var prefillTPS float64
					if prefillSamples > 0 {
						latenciesMutex.Lock()
						prefillTPS = sumPrefillTPS / float64(prefillSamples)
						latenciesMutex.Unlock()
					} else if avgTTFTMs > 0 && inTokens > 0 && success > 0 {
						// 备用计算：平均输入tokens / 平均首token延迟(秒)
						avgInTokens := float64(inTokens) / float64(success)
						avgTTFTSec := avgTTFTMs / 1000.0
						prefillTPS = avgInTokens / avgTTFTSec
					}

					var rps, tps, decodeTPSAvg float64
					decodeTimeNs := atomic.LoadInt64(&totalDecodeTime)
					// 使用有效生成时间计算 TPS
					effectiveDecodeSeconds := float64(decodeTimeNs) / float64(time.Second) / float64(concurrency)
					if effectiveDecodeSeconds > 0 {
						tps = float64(outTokens) / effectiveDecodeSeconds
						decodeTPSAvg = tps / float64(concurrency)
					}
					if elapsed > 0 {
						rps = float64(success) / elapsed
					}

					displayTotal := int(sent)
					if !isStressTestMode {
						displayTotal = totalRequestsTarget
					}

					e.progressCallback(&ProgressInfo{
						ModelName:       modelName,
						Concurrency:     concurrency,
						ContextTokens:   contextTargetTokens,
						CurrentRequests: int(success + failed),
						TotalRequests:   displayTotal,
						SuccessRequests: int(success),
						FailedRequests:  int(failed),
						AvgTTFTMs:       avgTTFTMs,
						PrefillTPS:      prefillTPS,
						RPS:             rps,
						TPS:             tps,
						DecodeTPSAvg:    decodeTPSAvg,
					})
				}
			}
		}
	}()

	// 用于通知 workers 停止发送新请求
	stopSending := make(chan struct{})
	var stopSendingClosed int32

	if isStressTestMode {
		// 时间模式（压力测试）：定时停止
		go func() {
			time.Sleep(e.config.Duration.Duration())
			if atomic.CompareAndSwapInt32(&stopSendingClosed, 0, 1) {
				close(stopSending)
				log.Printf("测试时间到，停止发送新请求")
			}
		}()
	}
	// 请求数模式：当成功返回数达到目标时停止（在请求成功处理后检查）

	// 启动固定数量的工作协程
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				// 检查是否应该停止
				select {
				case <-ctx.Done():
					return
				case <-stopSending:
					return
				default:
				}

				atomic.AddInt64(&requestsSent, 1)

				// 执行单个请求
				var reqCtx context.Context
				var cancel context.CancelFunc
				if e.config.RequestTimeout > 0 {
					reqCtx, cancel = context.WithTimeout(ctx, e.config.RequestTimeout.Duration())
				} else {
					reqCtx, cancel = context.WithCancel(ctx)
				}

				start := time.Now()
				var resp *model.LLMResponse
				var err error

				currentPrompt := e.getRandomPrompt()
				userMsg := currentPrompt
				if contextTargetTokens > 0 && contextText != "" {
					userMsg = fmt.Sprintf("[CONTEXT LEN=%d]\n%s\n\n任务：%s", contextTargetTokens, contextText, currentPrompt)
				}

				if mdl.SupportsVision() && (e.prompt.ImagePath != "" || e.prompt.ImageURL != "") {
					imagePath := e.prompt.ImagePath
					if imagePath == "" && e.prompt.ImageURL != "" {
						imagePath = e.prompt.ImageURL
					}
					resp, err = mdl.GenerateVisionResponse(reqCtx, e.prompt.SystemMessage, userMsg, imagePath, useStream)
				} else {
					resp, err = mdl.GenerateResponse(reqCtx, e.prompt.SystemMessage, userMsg, useStream)
				}
				latency := time.Since(start)
				cancel()

				latenciesMutex.Lock()
				latencies = append(latencies, latency)
				if err == nil && resp != nil && resp.TimeToFirstToken > 0 {
					firstTokenLatencies = append(firstTokenLatencies, resp.TimeToFirstToken)
					atomic.AddInt64(&totalTTFT, int64(resp.TimeToFirstToken))
					atomic.AddInt64(&ttftCount, 1)
				}
				latenciesMutex.Unlock()

				if err != nil {
					atomic.AddInt64(&failedCount, 1)
					errorsMutex.Lock()
					result.Errors = append(result.Errors, err.Error())
					errorsMutex.Unlock()
				} else {
					currentSuccess := atomic.AddInt64(&successCount, 1)
					endTime := time.Now() // 记录请求结束时间
					atomic.AddInt64(&totalLatency, int64(latency))
					atomic.AddInt64(&inputTokens, int64(resp.InputTokens))
					atomic.AddInt64(&outputTokens, int64(resp.OutputTokens))

					// 计算并累加生成时间（总延迟 - 首token延迟）
					decodeTime := latency
					if resp.TimeToFirstToken > 0 {
						decodeTime = latency - resp.TimeToFirstToken
						if decodeTime < 0 {
							decodeTime = latency // 保护性处理
						}
					}
					atomic.AddInt64(&totalDecodeTime, int64(decodeTime))

					// 收集时间信息，用于计算实际有效并发数
					latenciesMutex.Lock()
					allTimings = append(allTimings, requestTimingInfo{
						startTime:    start,
						ttft:         resp.TimeToFirstToken,
						endTime:      endTime,
						outputTokens: resp.OutputTokens,
					})
					latenciesMutex.Unlock()

					if resp.TimeToFirstToken > 0 {
						prefillSec := resp.TimeToFirstToken.Seconds()
						if prefillSec > 0 && resp.InputTokens > 0 {
							atomic.AddInt64(&numPrefillSamples, 1)
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

					if atomic.CompareAndSwapInt32(&firstResponsePrinted, 0, 1) {
						log.Printf("\n=== %s 第一次回复示例 ===", modelName)
						log.Printf("输入Token: %d, 输出Token: %d", resp.InputTokens, resp.OutputTokens)
						if resp.TimeToFirstToken > 0 {
							log.Printf("首Token延迟: %v", resp.TimeToFirstToken)
						}
						log.Printf("总延迟: %v", latency)
					}

					// 请求数模式：当成功数达到目标时，记录快照并停止发送
					if !isStressTestMode && currentSuccess == int64(totalRequestsTarget) {
						if atomic.CompareAndSwapInt32(&snapshotTaken, 0, 1) {
							snapshotTime = time.Now()
							snapshotOutputTokens = atomic.LoadInt64(&outputTokens)
							snapshotInputTokens = atomic.LoadInt64(&inputTokens)
							snapshotSuccessCount = currentSuccess
							snapshotFailedCount = atomic.LoadInt64(&failedCount)
							snapshotTotalDecodeTime = atomic.LoadInt64(&totalDecodeTime)
							snapshotTotalTTFT = atomic.LoadInt64(&totalTTFT)
							snapshotTTFTCount = atomic.LoadInt64(&ttftCount)
							latenciesMutex.Lock()
							snapshotPrefillTPS = sumPrefillTPS
							snapshotNumPrefillSamples = numPrefillSamples
							snapshotSumDecodeTPS = sumDecodeTPS
							snapshotNumDecodeSamples = numDecodeSamples
							// 复制当前的时间数据快照
							snapshotAllTimings = make([]requestTimingInfo, len(allTimings))
							copy(snapshotAllTimings, allTimings)
							latenciesMutex.Unlock()
							log.Printf("已达到目标请求数 %d，记录快照数据", totalRequestsTarget)
							// 通知所有 worker 停止发送新请求
							if atomic.CompareAndSwapInt32(&stopSendingClosed, 0, 1) {
								close(stopSending)
							}
						}
					}
				}
			}
		}()
	}

	// 等待所有工作协程完成
	log.Printf("等待所有请求完成...")
	wg.Wait()

	// 停止进度报告
	close(progressStop)
	<-progressDone

	// 计算总持续时间
	totalDuration := time.Since(startTime)
	log.Printf("所有请求已完成，总耗时: %v", totalDuration)

	// 请求数模式下使用快照数据（整个测试期间保持满并发）
	// 压力测试模式使用全部数据
	var finalOutputTokens, finalInputTokens, finalSuccessCount, finalFailedCount int64
	var finalTotalDecodeTime int64 // 累计生成时间
	var finalPrefillTPS float64
	var finalNumPrefillSamples int64
	var finalSumDecodeTPS float64
	var finalNumDecodeSamples int64
	var finalTotalTTFT int64
	var finalTTFTCount int64
	var effectiveDuration time.Duration

	if !isStressTestMode && snapshotTaken == 1 {
		// 请求数模式：使用快照数据
		finalOutputTokens = snapshotOutputTokens
		finalInputTokens = snapshotInputTokens
		finalSuccessCount = snapshotSuccessCount
		finalFailedCount = snapshotFailedCount
		finalTotalDecodeTime = snapshotTotalDecodeTime
		finalPrefillTPS = snapshotPrefillTPS
		finalNumPrefillSamples = snapshotNumPrefillSamples
		finalSumDecodeTPS = snapshotSumDecodeTPS
		finalNumDecodeSamples = snapshotNumDecodeSamples
		finalTotalTTFT = snapshotTotalTTFT
		finalTTFTCount = snapshotTTFTCount
		effectiveDuration = snapshotTime.Sub(startTime)
		log.Printf("使用快照数据计算结果，有效测试时长: %v", effectiveDuration)
	} else {
		// 压力测试模式：使用全部数据
		finalOutputTokens = outputTokens
		finalInputTokens = inputTokens
		finalSuccessCount = successCount
		finalFailedCount = failedCount
		finalTotalDecodeTime = totalDecodeTime
		finalTotalTTFT = totalTTFT
		finalTTFTCount = ttftCount
		latenciesMutex.Lock()
		finalPrefillTPS = sumPrefillTPS
		finalNumPrefillSamples = numPrefillSamples
		finalSumDecodeTPS = sumDecodeTPS
		finalNumDecodeSamples = numDecodeSamples
		latenciesMutex.Unlock()
		effectiveDuration = totalDuration
	}

	// 更新结果
	result.TotalRequests = int(finalSuccessCount) + int(finalFailedCount)
	result.SuccessRequests = int(finalSuccessCount)
	result.FailedRequests = int(finalFailedCount)
	result.TotalDuration = effectiveDuration
	result.ContextTargetTokens = contextTargetTokens

	if finalSuccessCount > 0 {
		// 计算有效生成时间 = 累计生成时间 / 并发数
		effectiveDecodeSeconds := float64(finalTotalDecodeTime) / float64(time.Second) / float64(concurrency)
		if effectiveDecodeSeconds <= 0 {
			// 如果没有有效的生成时间数据，回退到使用挂钟时间
			effectiveDecodeSeconds = effectiveDuration.Seconds()
		}

		result.AvgLatency = time.Duration(finalTotalDecodeTime / finalSuccessCount) // 平均生成时间
		result.InputTokens = finalInputTokens
		result.OutputTokens = finalOutputTokens
		result.TotalTokens = finalInputTokens + finalOutputTokens
		result.AvgInputTokens = float64(result.InputTokens) / float64(result.SuccessRequests)
		result.AvgOutputTokens = float64(result.OutputTokens) / float64(result.SuccessRequests)
		result.AvgTotalTokens = float64(result.TotalTokens) / float64(result.SuccessRequests)
		result.RequestsPerSec = float64(result.SuccessRequests) / effectiveDuration.Seconds()
		
		// 计算平均首token延迟（毫秒），用于判断是否发生排队
		var avgTTFTMs float64
		if finalTTFTCount > 0 {
			avgTTFTMs = float64(finalTotalTTFT) / float64(finalTTFTCount) / 1e6 // 纳秒转毫秒
		}
		
		// 计算实际有效并发数（通过时间重叠分析）
		// 请求数模式使用快照时的时间数据，压力测试模式使用全部数据
		var timingsForConcurrency []requestTimingInfo
		if !isStressTestMode && snapshotTaken == 1 && len(snapshotAllTimings) > 0 {
			timingsForConcurrency = snapshotAllTimings
		} else {
			timingsForConcurrency = allTimings
		}
		effectiveConcurrency := calculateEffectiveConcurrency(timingsForConcurrency)
		result.EffectiveConcurrency = effectiveConcurrency
		
		// 判断是否为流式模式（只有流式模式才能准确计算实际并发数）
		canCalculateEffectiveConcurrency := useStream && effectiveConcurrency > 0
		
		if canCalculateEffectiveConcurrency {
			// 流式模式：使用实际有效并发数计算TPS
			// 公式：TPS = 单请求平均Decode速度 × 实际有效并发数
			if finalNumDecodeSamples > 0 {
				result.DecodeTokensPerSecAvg = finalSumDecodeTPS / float64(finalNumDecodeSamples)
			}
			result.TokensPerSec = result.DecodeTokensPerSecAvg * effectiveConcurrency
			
			log.Printf("TPS计算(流式-实际并发): 平均TTFT=%.0fms", avgTTFTMs)
			log.Printf("  实际有效并发: %.1f (发送并发: %d)", effectiveConcurrency, concurrency)
			log.Printf("  单请求Decode TPS=%.2f, 总TPS=%.2f", 
				result.DecodeTokensPerSecAvg, result.TokensPerSec)
		} else {
			// 非流式模式：使用传统方法（可能不准确）
			// 公式：TPS = 总输出tokens / (累计生成时间 / 并发数)
			result.TokensPerSec = float64(result.OutputTokens) / effectiveDecodeSeconds
			if concurrency > 0 {
				result.DecodeTokensPerSecAvg = result.TokensPerSec / float64(concurrency)
			}
			
			if !useStream {
				log.Printf("警告：非流式模式无法准确测量实际并发，TPS可能不准确")
				log.Printf("  建议开启流式模式(stream: true)以获取准确的TPS数据")
			}
			log.Printf("TPS计算(传统方法): 平均TTFT=%.0fms, 输出tokens=%d, 有效时间=%.2fs", 
				avgTTFTMs, result.OutputTokens, effectiveDecodeSeconds)
			log.Printf("  总TPS=%.2f, 单请求Decode TPS=%.2f (基于发送并发%d)", 
				result.TokensPerSec, result.DecodeTokensPerSecAvg, concurrency)
		}

		if finalNumPrefillSamples > 0 {
			result.PrefillTokensPerSecAvg = finalPrefillTPS / float64(finalNumPrefillSamples)
		}
	}

	result.AllLatencies = latencies
	result.FirstTokenLatencies = firstTokenLatencies

	if len(latencies) > 0 && len(e.config.LatencyPercentiles) > 0 {
		result.LatencyPercentiles = make(map[int]time.Duration)
		for _, p := range e.config.LatencyPercentiles {
			result.LatencyPercentiles[p] = calculatePercentile(latencies, p)
		}
	}

	if len(firstTokenLatencies) > 0 {
		var totalFirstTokenLatency int64
		for _, ftl := range firstTokenLatencies {
			totalFirstTokenLatency += int64(ftl)
		}
		result.AvgFirstTokenLatency = time.Duration(totalFirstTokenLatency / int64(len(firstTokenLatencies)))

		if len(e.config.LatencyPercentiles) > 0 {
			result.FirstTokenPercentiles = make(map[int]time.Duration)
			for _, p := range e.config.LatencyPercentiles {
				result.FirstTokenPercentiles[p] = calculatePercentile(firstTokenLatencies, p)
			}
		}
	}

	// 发送最终进度
	if e.progressCallback != nil {
		var avgTTFTMs float64
		if result.AvgFirstTokenLatency > 0 {
			avgTTFTMs = float64(result.AvgFirstTokenLatency.Milliseconds())
		}
		e.progressCallback(&ProgressInfo{
			ModelName:       modelName,
			Concurrency:     concurrency,
			ContextTokens:   contextTargetTokens,
			CurrentRequests: result.TotalRequests,
			TotalRequests:   result.TotalRequests,
			SuccessRequests: result.SuccessRequests,
			FailedRequests:  result.FailedRequests,
			AvgTTFTMs:       avgTTFTMs,
			RPS:             result.RequestsPerSec,
			TPS:             result.TokensPerSec,
			DecodeTPSAvg:    result.DecodeTokensPerSecAvg,
		})
	}

	return nil
}
