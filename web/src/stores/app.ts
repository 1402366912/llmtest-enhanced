import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Machine, Backend, Config, TestProgress, TestResult, HistoryRecord, LeaderboardEntry } from '@/types'
import { machineApi, backendApi, configApi, testApi, historyApi, leaderboardApi, createWebSocketForRun, closeWebSocketForRun, closeAllWebSockets } from '@/api'

export const useAppStore = defineStore('app', () => {
  // 状态
  const machines = ref<Machine[]>([])
  const currentMachineId = ref<string>('')
  const backends = ref<Backend[]>([])
  const currentBackendId = ref<string>('')
  const config = ref<Config | null>(null)
  const testRunning = ref(false)
  const testProgress = ref<TestProgress | null>(null)
  const testResults = ref<Record<string, TestResult> | null>(null)
  const historyRecords = ref<HistoryRecord[]>([])
  const leaderboard = ref<LeaderboardEntry[]>([])
  const leaderboardModels = ref<string[]>([])
  const leaderboardConcurrencies = ref<number[]>([])
  const leaderboardContextTokens = ref<number[]>([])
  const leaderboardFilter = ref<{ concurrency: number; context_tokens: number; model: string }>({ concurrency: 0, context_tokens: 0, model: '' })
  const machineBackends = ref<Record<string, Backend[]>>({}) // 所有机器的后端列表缓存
  const loading = ref(false)
  const error = ref<string | null>(null)
  const wsConnected = ref(false)
  
  // 当前运行的测试 ID
  const currentRunId = ref<string>('')
  const currentTestModelName = ref<string>('') // 当前正在测试的模型名称
  
  // WebSocket (per run)
  let ws: WebSocket | null = null
  
  // 计算属性
  const currentMachine = computed(() => 
    machines.value.find(m => m.id === currentMachineId.value) || null
  )
  
  const currentBackend = computed(() => 
    backends.value.find(b => b.id === currentBackendId.value) || null
  )
  
  const enabledModels = computed(() => 
    config.value?.models.filter(m => !m.skip) || []
  )
  
  // Actions
  
  // 初始化
  async function initialize() {
    loading.value = true
    error.value = null
    
    try {
      // 加载机器列表
      await loadMachines()
      
      // 如果有当前机器，加载后端列表
      if (currentMachineId.value) {
        await loadBackends(currentMachineId.value)
      }
      
      // 加载当前配置
      await loadConfig()
      
      // 注意：不再在这里初始化 WebSocket，WebSocket 会在启动测试时创建
    } catch (e: any) {
      error.value = e.message || '初始化失败'
    } finally {
      loading.value = false
    }
  }
  
  // 加载机器列表
  async function loadMachines() {
    const res = await machineApi.getList()
    if (res.success && res.data) {
      machines.value = res.data.machines
      currentMachineId.value = res.data.current_machine
    }
  }
  
  // 创建机器
  async function createMachine(data: { name: string; description?: string; gpu_model?: string }) {
    const res = await machineApi.create(data)
    if (res.success) {
      await loadMachines()
      return true
    }
    throw new Error(res.error || '创建失败')
  }
  
  // 更新机器
  async function updateMachine(id: string, data: { name?: string; description?: string; gpu_model?: string }) {
    const res = await machineApi.update(id, data)
    if (res.success) {
      await loadMachines()
      return true
    }
    throw new Error(res.error || '更新失败')
  }
  
  // 删除机器
  async function deleteMachine(id: string) {
    const res = await machineApi.delete(id)
    if (res.success) {
      await loadMachines()
      return true
    }
    throw new Error(res.error || '删除失败')
  }
  
  // 切换机器
  async function selectMachine(id: string) {
    const res = await machineApi.select(id)
    if (res.success) {
      currentMachineId.value = id
      // 加载该机器的后端列表
      await loadBackends(id)
      await loadConfig()
      await loadHistory()
      return true
    }
    throw new Error(res.error || '切换失败')
  }
  
  // 加载后端列表
  async function loadBackends(machineId?: string) {
    const mid = machineId || currentMachineId.value
    if (!mid) {
      backends.value = []
      currentBackendId.value = ''
      return
    }
    const res = await backendApi.getList(mid)
    if (res.success && res.data) {
      backends.value = res.data.backends
      currentBackendId.value = res.data.current_backend
      // 同时更新缓存
      machineBackends.value[mid] = res.data.backends
    }
  }
  
  // 加载指定机器的后端列表（用于对比分析）
  async function loadBackendsForMachine(machineId: string) {
    if (machineBackends.value[machineId]) {
      return machineBackends.value[machineId]
    }
    const res = await backendApi.getList(machineId)
    if (res.success && res.data) {
      machineBackends.value[machineId] = res.data.backends
      return res.data.backends
    }
    return []
  }
  
  // 创建后端
  async function createBackend(data: { name: string; description?: string }) {
    if (!currentMachineId.value) throw new Error('请先选择机器')
    const res = await backendApi.create(currentMachineId.value, data)
    if (res.success) {
      await loadBackends()
      return true
    }
    throw new Error(res.error || '创建失败')
  }
  
  // 更新后端
  async function updateBackend(backendId: string, data: { name?: string; description?: string }) {
    if (!currentMachineId.value) throw new Error('请先选择机器')
    const res = await backendApi.update(currentMachineId.value, backendId, data)
    if (res.success) {
      await loadBackends()
      return true
    }
    throw new Error(res.error || '更新失败')
  }
  
  // 删除后端
  async function deleteBackend(backendId: string) {
    if (!currentMachineId.value) throw new Error('请先选择机器')
    const res = await backendApi.delete(currentMachineId.value, backendId)
    if (res.success) {
      await loadBackends()
      return true
    }
    throw new Error(res.error || '删除失败')
  }
  
  // 切换后端
  async function selectBackend(backendId: string) {
    if (!currentMachineId.value) throw new Error('请先选择机器')
    const res = await backendApi.select(currentMachineId.value, backendId)
    if (res.success) {
      currentBackendId.value = backendId
      await loadConfig()
      await loadHistory()
      return true
    }
    throw new Error(res.error || '切换失败')
  }
  
  // 加载配置
  async function loadConfig() {
    const res = await configApi.get()
    if (res.success && res.data) {
      config.value = res.data
    }
  }
  
  // 保存配置
  async function saveConfig(newConfig: Config) {
    const res = await configApi.update(newConfig)
    if (res.success) {
      config.value = newConfig
      return true
    }
    throw new Error(res.error || '保存失败')
  }
  
  // 检查测试状态
  async function checkTestStatus() {
    if (!currentRunId.value) {
      testRunning.value = false
      return
    }
    const res = await testApi.getStatus(currentRunId.value)
    if (res.success && res.data) {
      testRunning.value = res.data.running
    }
  }
  
  // 开始测试（需要指定模型名称）
  async function startTest(modelName: string) {
    if (!currentMachineId.value || !currentBackendId.value) {
      throw new Error('请先选择机器和后端')
    }
    if (!modelName) {
      throw new Error('请选择要测试的模型')
    }
    
    const res = await testApi.start({
      machine_id: currentMachineId.value,
      backend_id: currentBackendId.value,
      model_name: modelName
    })
    
    if (res.success && res.data) {
      currentRunId.value = res.data.run_id
      currentTestModelName.value = modelName
      testRunning.value = true
      testProgress.value = null
      testResults.value = null
      
      // 建立 WebSocket 连接
      initWebSocketForRun(res.data.run_id)
      
      return res.data.run_id
    }
    throw new Error(res.error || '启动失败')
  }
  
  // 停止测试
  async function stopTest() {
    if (!currentRunId.value) {
      throw new Error('没有正在运行的测试')
    }
    
    const res = await testApi.stop(currentRunId.value)
    if (res.success) {
      return true
    }
    throw new Error(res.error || '停止失败')
  }
  
  // 获取测试结果
  async function loadTestResults() {
    if (!currentRunId.value) {
      testResults.value = null
      return
    }
    
    const res = await testApi.getResults(currentRunId.value)
    if (res.success && res.data) {
      testResults.value = res.data.results
    }
  }
  
  // 初始化特定 Run 的 WebSocket
  function initWebSocketForRun(runId: string) {
    // 关闭之前的连接
    if (ws) {
      closeWebSocketForRun(currentRunId.value)
    }
    
    ws = createWebSocketForRun(
      runId,
      (data: TestProgress) => {
        handleWebSocketMessage(data)
      },
      (connected: boolean) => {
        wsConnected.value = connected
        if (connected) {
          error.value = null
        }
      }
    )
  }
  
  // 清理当前 Run
  function cleanupCurrentRun() {
    if (currentRunId.value) {
      closeWebSocketForRun(currentRunId.value)
    }
    currentRunId.value = ''
    currentTestModelName.value = ''
    testRunning.value = false
    testProgress.value = null
    wsConnected.value = false
    ws = null
  }
  
  // 加载历史记录
  async function loadHistory(machineId?: string, backendId?: string) {
    const mid = machineId || currentMachineId.value
    const bid = backendId || currentBackendId.value
    const res = await historyApi.getList(mid, bid)
    if (res.success && res.data) {
      historyRecords.value = res.data
    }
  }
  
  // 删除历史记录
  async function deleteHistory(id: string, machineId?: string, backendId?: string) {
    const mid = machineId || currentMachineId.value
    const bid = backendId || currentBackendId.value
    const res = await historyApi.delete(id, mid, bid)
    if (res.success) {
      await loadHistory()
      return true
    }
    throw new Error(res.error || '删除失败')
  }
  
  // 加载天梯图
  async function loadLeaderboard(params?: { metric?: 'tps' | 'rps' | 'latency' | 'ttft'; model?: string; concurrency?: number; context_tokens?: number }) {
    const res = await leaderboardApi.get(params) as any
    if (res.success) {
      // 后端直接返回平铺格式，data 字段直接是数组
      leaderboard.value = res.data || []
      leaderboardModels.value = res.available_models || []
      leaderboardConcurrencies.value = res.available_concurrencies || []
      leaderboardContextTokens.value = res.available_context_tokens || []
      if (res.filter) {
        leaderboardFilter.value = {
          concurrency: res.filter.concurrency || 0,
          context_tokens: res.filter.context_tokens || 0,
          model: res.filter.model || ''
        }
      }
    }
  }
  
  // 处理WebSocket消息
  function handleWebSocketMessage(data: TestProgress) {
    switch (data.type) {
      case 'status':
        testRunning.value = data.running || false
        break
      case 'start':
        testRunning.value = true
        testProgress.value = data
        testResults.value = null
        break
      case 'progress':
        testProgress.value = data
        break
      case 'complete':
        testRunning.value = false
        testProgress.value = null
        loadTestResults()
        loadHistory()
        break
      case 'stopped':
        testRunning.value = false
        testProgress.value = null
        break
      case 'error':
        testRunning.value = false
        error.value = data.message || '测试出错'
        break
    }
  }
  
  return {
    // State
    machines,
    currentMachineId,
    currentMachine,
    backends,
    currentBackendId,
    currentBackend,
    config,
    testRunning,
    testProgress,
    testResults,
    historyRecords,
    leaderboard,
    leaderboardModels,
    leaderboardConcurrencies,
    leaderboardContextTokens,
    leaderboardFilter,
    machineBackends,
    loading,
    error,
    enabledModels,
    wsConnected,
    currentRunId,
    currentTestModelName,
    
    // Actions
    initialize,
    loadMachines,
    createMachine,
    updateMachine,
    deleteMachine,
    selectMachine,
    loadBackends,
    loadBackendsForMachine,
    createBackend,
    updateBackend,
    deleteBackend,
    selectBackend,
    loadConfig,
    saveConfig,
    startTest,
    stopTest,
    loadTestResults,
    loadHistory,
    deleteHistory,
    loadLeaderboard,
    cleanupCurrentRun,
  }
})

