// 机器类型
export interface Machine {
  id: string
  name: string
  description?: string
  gpu_model?: string
  created_at: string
  updated_at: string
}

// 后端类型
export interface Backend {
  id: string
  name: string
  description?: string
  created_at: string
  updated_at: string
}

// 模型配置
export interface ModelConfig {
  name: string
  type: string
  api_key: string
  base_url: string
  params: Record<string, any>
  tokenizer_path?: string
  skip: boolean
  concurrency_levels?: number[]
  stream?: boolean
  proxy_name?: string
}

// 测试配置
export interface TestConfig {
  concurrency: number
  stress_test_mode: boolean        // 压力测试模式（true: 按时间测试，false: 按请求数测试，请求数=并发×3）
  duration: string                 // 测试持续时间（仅在stress_test_mode=true时使用）
  warmup_duration: string
  request_timeout: string
  concurrency_levels: number[]
  show_progress: boolean
  max_retries: number
  latency_percentiles: number[]
  context_token_levels?: number[]
  context_token_start?: number
  context_token_end?: number
  context_tolerance?: number
  enable_prefill_metrics?: boolean
}

// 提示词配置
export interface PromptConfig {
  system_message: string
  user_message: string
  dataset_path?: string
  stream: boolean
  image_path?: string
  image_url?: string
  context_base?: string
}

// 代理配置
export interface ProxyConfig {
  name: string
  url: string
}

// 完整配置
export interface Config {
  test: TestConfig
  models: ModelConfig[]
  prompt: PromptConfig
  proxies?: ProxyConfig[]
}

// 测试结果
export interface TestResult {
  ModelName: string
  ConcurrencyLevel: number
  ContextTargetTokens: number
  TotalRequests: number
  SuccessRequests: number
  FailedRequests: number
  TotalDuration: number
  AvgLatency: number
  InputTokens: number
  OutputTokens: number
  TotalTokens: number
  AvgInputTokens: number
  AvgOutputTokens: number
  AvgTotalTokens: number
  RequestsPerSec: number
  TokensPerSec: number
  PrefillTokensPerSecAvg: number
  DecodeTokensPerSecAvg: number
  Errors: string[]
  LatencyPercentiles: Record<number, number>
  AllLatencies: number[]
  FirstTokenLatencies: number[]
  AvgFirstTokenLatency: number
  FirstTokenPercentiles: Record<number, number>
}

// 测试进度
export interface TestProgress {
  type: 'start' | 'progress' | 'result' | 'error' | 'complete' | 'stopped' | 'status'
  model_name?: string
  concurrency?: number
  context_tokens?: number
  current_requests?: number
  total_requests?: number
  success_requests?: number
  failed_requests?: number
  avg_ttft_ms?: number       // 平均首Token延迟(毫秒)
  prefill_tps?: number       // 预填充速度 (输入tokens/首token延迟)
  rps?: number
  tps?: number               // 总体TPS
  decode_tps_avg?: number    // 平均生成速度
  message?: string
  running?: boolean
}

// 历史记录
export interface HistoryRecord {
  id: string
  machine_id: string
  machine_name?: string
  backend_id?: string
  backend_name?: string
  file_name: string
  file_path: string
  created_at: string
  summary?: HistorySummary
}

export interface HistorySummary {
  total_requests: number
  success_requests: number
  model_count: number
  max_tps: number
  max_rps: number
}

// 天梯图条目
export interface LeaderboardEntry {
  rank: number
  machine_id: string
  machine_name: string
  backend_id: string
  backend_name: string
  gpu_model: string
  model_name: string
  concurrency: number
  context_tokens: number
  total_requests: number
  tps: number
  rps: number
  avg_latency_ms: number
  ttft_ms: number
  success_rate: number
}

// API 响应
export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

