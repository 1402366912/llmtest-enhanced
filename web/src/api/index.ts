import axios from 'axios'
import type { 
  Machine, 
  Backend,
  Config, 
  ModelConfig, 
  TestResult, 
  HistoryRecord, 
  LeaderboardEntry,
  ApiResponse 
} from '@/types'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

// 响应拦截器
api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    console.error('API Error:', error)
    return Promise.reject(error)
  }
)

// 机器管理 API
export const machineApi = {
  // 获取机器列表
  getList: (): Promise<ApiResponse<{ machines: Machine[]; current_machine: string }>> =>
    api.get('/machines'),
  
  // 创建机器
  create: (data: { name: string; description?: string; gpu_model?: string }): Promise<ApiResponse<Machine>> =>
    api.post('/machines', data),
  
  // 更新机器
  update: (id: string, data: { name?: string; description?: string; gpu_model?: string }): Promise<ApiResponse<void>> =>
    api.put(`/machines/${id}`, data),
  
  // 删除机器
  delete: (id: string): Promise<ApiResponse<void>> =>
    api.delete(`/machines/${id}`),
  
  // 选择/切换机器
  select: (id: string): Promise<ApiResponse<{ machine_id: string }>> =>
    api.post(`/machines/${id}/select`),
  
  // 获取当前机器
  getCurrent: (): Promise<ApiResponse<Machine | null>> =>
    api.get('/machines/current'),
  
  // 获取机器配置
  getConfig: (id: string): Promise<ApiResponse<Config>> =>
    api.get(`/machines/${id}/config`),
  
  // 更新机器配置
  updateConfig: (id: string, config: Config): Promise<ApiResponse<void>> =>
    api.put(`/machines/${id}/config`, config),
}

// 后端管理 API
export const backendApi = {
  // 获取后端列表
  getList: (machineId: string): Promise<ApiResponse<{ backends: Backend[]; current_backend: string }>> =>
    api.get(`/machines/${machineId}/backends`),
  
  // 创建后端
  create: (machineId: string, data: { name: string; description?: string }): Promise<ApiResponse<Backend>> =>
    api.post(`/machines/${machineId}/backends`, data),
  
  // 更新后端
  update: (machineId: string, backendId: string, data: { name?: string; description?: string }): Promise<ApiResponse<void>> =>
    api.put(`/machines/${machineId}/backends/${backendId}`, data),
  
  // 删除后端
  delete: (machineId: string, backendId: string): Promise<ApiResponse<void>> =>
    api.delete(`/machines/${machineId}/backends/${backendId}`),
  
  // 选择/切换后端
  select: (machineId: string, backendId: string): Promise<ApiResponse<{ machine_id: string; backend_id: string }>> =>
    api.post(`/machines/${machineId}/backends/${backendId}/select`),
  
  // 获取当前后端
  getCurrent: (machineId: string): Promise<ApiResponse<Backend | null>> =>
    api.get(`/machines/${machineId}/backends/current`),
  
  // 获取后端配置
  getConfig: (machineId: string, backendId: string): Promise<ApiResponse<Config>> =>
    api.get(`/machines/${machineId}/backends/${backendId}/config`),
  
  // 更新后端配置
  updateConfig: (machineId: string, backendId: string, config: Config): Promise<ApiResponse<void>> =>
    api.put(`/machines/${machineId}/backends/${backendId}/config`, config),
}

// 配置 API
export const configApi = {
  // 获取当前配置
  get: (): Promise<ApiResponse<Config>> =>
    api.get('/config'),
  
  // 更新配置
  update: (config: Config): Promise<ApiResponse<void>> =>
    api.put('/config', config),
  
  // 重新加载配置
  reload: (): Promise<ApiResponse<void>> =>
    api.post('/config/reload'),
}

// 数据集预览结果
export interface DatasetPreview {
  path: string
  exists: boolean
  total_count: number
  samples: string[]
  error?: string
}

// 数据集 API
export const datasetApi = {
  // 预览数据集
  preview: (): Promise<ApiResponse<DatasetPreview>> =>
    api.get('/dataset/preview'),
}

// 模型 API
export const modelApi = {
  // 获取模型列表
  getList: (): Promise<ApiResponse<ModelConfig[]>> =>
    api.get('/models'),
  
  // 添加模型
  add: (model: ModelConfig): Promise<ApiResponse<void>> =>
    api.post('/models', model),
  
  // 更新模型
  update: (name: string, model: ModelConfig): Promise<ApiResponse<void>> =>
    api.put(`/models/${name}`, model),
  
  // 删除模型
  delete: (name: string): Promise<ApiResponse<void>> =>
    api.delete(`/models/${name}`),
  
  // 切换模型启用状态
  toggle: (name: string): Promise<ApiResponse<{ skip: boolean }>> =>
    api.put(`/models/${name}/toggle`),
}

// 连接测试结果
export interface ConnectionTestResult {
  success: boolean
  model_name: string
  message: string
  latency_ms: number
  token_count?: number
  response?: string
}

// 启动测试请求
export interface StartTestRequest {
  machine_id: string
  backend_id: string
  model_name: string
}

// 启动测试响应
export interface StartTestResponse {
  run_id: string
}

// 测试状态响应
export interface TestStatusResponse {
  run_id?: string
  running: boolean
  machine_id?: string
  backend_id?: string
  model_name?: string
  start_time?: string
  runs?: Array<{
    run_id: string
    running: boolean
    machine_id: string
    backend_id: string
    model_name: string
    start_time: string
  }>
}

// 测试 API
export const testApi = {
  // 开始测试（需要指定机器、后端和模型）
  start: (params: StartTestRequest): Promise<ApiResponse<StartTestResponse>> =>
    api.post('/test/start', params),
  
  // 停止测试（需要指定 run_id）
  stop: (runId: string): Promise<ApiResponse<void>> =>
    api.post('/test/stop', { run_id: runId }),
  
  // 获取测试状态（可选 run_id）
  getStatus: (runId?: string): Promise<ApiResponse<TestStatusResponse>> =>
    api.get('/test/status', { params: runId ? { run_id: runId } : {} }),
  
  // 获取测试结果（需要指定 run_id）
  getResults: (runId: string): Promise<ApiResponse<{
    results: Record<string, TestResult>
    start_time: string
    end_time: string
    duration: string
  } | null>> =>
    api.get('/test/results', { params: { run_id: runId } }),
  
  // 测试模型连接
  testConnection: (modelName: string): Promise<ApiResponse<ConnectionTestResult>> =>
    api.post('/test/connection', { model_name: modelName }),
}

// 历史记录 API
export const historyApi = {
  // 获取历史记录列表
  getList: (machineId?: string, backendId?: string): Promise<ApiResponse<HistoryRecord[]>> =>
    api.get('/history', { params: { machine_id: machineId, backend_id: backendId } }),
  
  // 获取历史记录详情
  getDetail: (id: string, machineId: string, backendId?: string): Promise<ApiResponse<Record<string, TestResult>>> =>
    api.get(`/history/${id}`, { params: { machine_id: machineId, backend_id: backendId } }),
  
  // 删除历史记录
  delete: (id: string, machineId: string, backendId?: string): Promise<ApiResponse<void>> =>
    api.delete(`/history/${id}`, { params: { machine_id: machineId, backend_id: backendId } }),
  
  // 导出历史记录
  exportUrl: (id: string, machineId: string, backendId?: string): string =>
    `/api/history/${id}/export?machine_id=${machineId}${backendId ? `&backend_id=${backendId}` : ''}`,
}

// 天梯图响应
export interface LeaderboardResponse {
  data: LeaderboardEntry[]
  available_models: string[]
  available_concurrencies: number[]
  available_context_tokens: number[]
  metric: string
  filter: {
    concurrency: number
    context_tokens: number
    model: string
  }
}

// 天梯图筛选参数
export interface LeaderboardParams {
  metric?: 'tps' | 'rps' | 'latency' | 'ttft'
  model?: string
  concurrency?: number
  context_tokens?: number
}

// 天梯图 API
export const leaderboardApi = {
  // 获取天梯图数据
  get: (params?: LeaderboardParams): Promise<ApiResponse<LeaderboardResponse>> =>
    api.get('/leaderboard', { params }),
}

// 分析数据点
export interface AnalyticsDataPoint {
  concurrency: number
  context_tokens: number
  tps: number
  rps: number
  avg_latency_ms: number
  success_rate: number
}

// 分析数据
export interface AnalyticsData {
  machine_id: string
  machine_name: string
  backend_id: string
  backend_name: string
  model_name: string
  data: AnalyticsDataPoint[]
}

// 数据分析 API
export const analyticsApi = {
  // 获取指定机器/后端/模型的分析数据
  getData: (machineId: string, backendId: string, model?: string): Promise<ApiResponse<AnalyticsData>> =>
    api.get('/analytics', { params: { machine_id: machineId, backend_id: backendId, model } }),
  
  // 获取所有可用模型列表
  getModels: (): Promise<ApiResponse<string[]>> =>
    api.get('/analytics/models'),
}

// 导入结果
export interface ImportResult {
  machines_added: number
  machines_merged: number
  backends_added: number
  backends_merged: number
  results_added: number
  results_skipped: number
  errors?: string[]
}

// 数据导入导出 API
export const dataTransferApi = {
  // 导出数据（直接下载ZIP）
  exportData: (customName?: string) => {
    const url = customName 
      ? `/api/export?name=${encodeURIComponent(customName)}`
      : '/api/export'
    window.location.href = url
  },
  
  // 导入数据
  importData: (file: File): Promise<ApiResponse<ImportResult>> => {
    const formData = new FormData()
    formData.append('file', file)
    return api.post('/import', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 60000, // 导入可能需要更长时间
    })
  },
}

// WebSocket 连接管理器（按 runId 管理多个连接）
const wsInstances: Map<string, WebSocket> = new Map()
const wsReconnectTimers: Map<string, ReturnType<typeof setTimeout>> = new Map()

export function createWebSocketForRun(
  runId: string,
  onMessage: (data: any) => void, 
  onStatusChange?: (connected: boolean) => void
): WebSocket {
  // 清理之前同 runId 的连接
  closeWebSocketForRun(runId)

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const ws = new WebSocket(`${protocol}//${host}/api/ws?run_id=${runId}`)
  wsInstances.set(runId, ws)
  
  ws.onopen = () => {
    console.log(`WebSocket connected for run ${runId}`)
    onStatusChange?.(true)
  }
  
  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      onMessage(data)
    } catch (e) {
      console.error('WebSocket message parse error:', e)
    }
  }
  
  ws.onerror = (error) => {
    console.error(`WebSocket error for run ${runId}:`, error)
    onStatusChange?.(false)
  }
  
  ws.onclose = () => {
    console.log(`WebSocket closed for run ${runId}`)
    onStatusChange?.(false)
    // 不自动重连，由调用方决定是否重连
  }
  
  return ws
}

export function closeWebSocketForRun(runId: string) {
  const timer = wsReconnectTimers.get(runId)
  if (timer) {
    clearTimeout(timer)
    wsReconnectTimers.delete(runId)
  }
  const ws = wsInstances.get(runId)
  if (ws) {
    ws.close()
    wsInstances.delete(runId)
  }
}

export function closeAllWebSockets() {
  wsReconnectTimers.forEach((timer) => clearTimeout(timer))
  wsReconnectTimers.clear()
  wsInstances.forEach((ws) => ws.close())
  wsInstances.clear()
}

// 兼容旧接口（不再使用，保留以防其他地方引用）
export function createWebSocket(
  onMessage: (data: any) => void, 
  onStatusChange?: (connected: boolean) => void
): WebSocket | null {
  console.warn('createWebSocket is deprecated, use createWebSocketForRun instead')
  return null as any
}

export function closeWebSocket() {
  console.warn('closeWebSocket is deprecated, use closeWebSocketForRun or closeAllWebSockets instead')
  closeAllWebSockets()
}
