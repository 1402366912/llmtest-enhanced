<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { 
  NCard, NSelect, NSpace, NEmpty, NIcon, NGrid, NGi, NAlert, NSpin,
  NRadioGroup, NRadioButton, NTag, NCascader, type CascaderOption
} from 'naive-ui'
import { AnalyticsOutline, CloseCircleOutline } from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import { historyApi, analyticsApi, type AnalyticsData, type AnalyticsDataPoint } from '@/api'
import type { TestResult, Backend } from '@/types'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { TitleComponent, TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'

use([CanvasRenderer, LineChart, TitleComponent, TooltipComponent, GridComponent, LegendComponent])

const store = useAppStore()

// 分析模式
type AnalysisMode = 'single' | 'compare'
const analysisMode = ref<AnalysisMode>('single')

// ========== 单机分析模式状态 ==========
const loading = ref(false)
const selectedModel = ref<string>('')
const selectedHistoryId = ref<string>('')
const testResults = ref<Record<string, TestResult>>({})

// 单机分析固定维度筛选
const fixedContextForConcurrencyChart = ref<number | null>(null) // 用于并发图：固定上下文
const fixedConcurrencyForContextChart = ref<number | null>(null) // 用于上下文图：固定并发

// ========== 对比分析模式状态 ==========
const compareLoading = ref(false)
const compareModel = ref<string>('')
const availableModels = ref<string[]>([])
const selectedBackends = ref<string[]>([]) // 格式: "machineId/backendId"
const compareData = ref<AnalyticsData[]>([])

// 图表颜色
const chartColors = ['#6366f1', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#84cc16']

// ========== 单机分析模式逻辑 ==========

// 模型选项（从测试结果中提取）
const modelOptions = computed(() => {
  const models = new Set<string>()
  Object.values(testResults.value).forEach(r => {
    models.add(r.ModelName)
  })
  return Array.from(models).map(m => ({ label: m, value: m }))
})

// 历史记录选项
const historyOptions = computed(() => {
  return store.historyRecords.map(h => ({
    label: `${h.file_name} (${new Date(h.created_at).toLocaleString()})`,
    value: h.id
  }))
})

// 加载历史记录列表
async function loadHistoryList() {
  if (!store.currentMachineId) return
  await store.loadHistory()
  const firstRecord = store.historyRecords[0]
  if (firstRecord && !selectedHistoryId.value) {
    selectedHistoryId.value = firstRecord.id
  }
}

// 加载选中的历史记录详情
async function loadHistoryDetail() {
  if (!selectedHistoryId.value || !store.currentMachineId) return
  
  loading.value = true
  try {
    const res = await historyApi.getDetail(
      selectedHistoryId.value, 
      store.currentMachineId,
      store.currentBackendId || undefined
    )
    if (res.success && res.data) {
      testResults.value = res.data
      const firstModel = modelOptions.value[0]
      if (firstModel && !selectedModel.value) {
        selectedModel.value = firstModel.value
      }
    }
  } catch (e) {
    console.error('加载历史详情失败:', e)
  } finally {
    loading.value = false
  }
}

// 过滤当前模型的结果
const filteredResults = computed(() => {
  if (!selectedModel.value) return []
  return Object.values(testResults.value).filter(r => r.ModelName === selectedModel.value)
})

// 可用的上下文长度列表（用于筛选下拉框）
const availableContextLengths = computed(() => {
  const contexts = [...new Set(filteredResults.value.map(r => r.ContextTargetTokens))].sort((a, b) => a - b)
  return contexts.map(c => ({ label: `${c} tokens`, value: c }))
})

// 可用的并发数列表（用于筛选下拉框）
const availableConcurrencies = computed(() => {
  const concurrencies = [...new Set(filteredResults.value.map(r => r.ConcurrencyLevel))].sort((a, b) => a - b)
  return concurrencies.map(c => ({ label: `${c} 并发`, value: c }))
})

// 当模型改变时，重置筛选并设置默认值
watch(selectedModel, () => {
  // 固定上下文默认取最小非零值
  const contexts = [...new Set(filteredResults.value.map(r => r.ContextTargetTokens))].sort((a, b) => a - b)
  const minContext = contexts.find(c => c > 0) || contexts[0]
  fixedContextForConcurrencyChart.value = minContext || null
  
  // 固定并发默认取 10（若存在）否则取最小值
  const concurrencies = [...new Set(filteredResults.value.map(r => r.ConcurrencyLevel))].sort((a, b) => a - b)
  fixedConcurrencyForContextChart.value = concurrencies.includes(10) ? 10 : (concurrencies[0] || null)
})

// ========== 对比分析模式逻辑 ==========

// 后端级联选择器选项
const backendCascaderOptions = computed<CascaderOption[]>(() => {
  return store.machines.map(machine => ({
    label: machine.name,
    value: machine.id,
    children: (store.machineBackends[machine.id] || []).map(backend => ({
      label: backend.name,
      value: `${machine.id}/${backend.id}`
    }))
  }))
})

// 加载所有机器的后端列表
async function loadAllBackends() {
  for (const machine of store.machines) {
    if (!store.machineBackends[machine.id]) {
      try {
        await store.loadBackendsForMachine(machine.id)
      } catch (e) {
        console.error(`加载机器 ${machine.name} 的后端失败:`, e)
      }
    }
  }
}

// 加载可用模型列表
async function loadAvailableModels() {
  try {
    const res = await analyticsApi.getModels()
    if (res.success && res.data) {
      availableModels.value = res.data
    }
  } catch (e) {
    console.error('加载模型列表失败:', e)
  }
}

// 加载对比数据
async function loadCompareData() {
  if (!compareModel.value || selectedBackends.value.length === 0) {
    compareData.value = []
    return
  }
  
  compareLoading.value = true
  try {
    const promises = selectedBackends.value.map(async (key) => {
      const parts = key.split('/')
      const machineId = parts[0] || ''
      const backendId = parts[1] || ''
      if (!machineId || !backendId) return null
      const res = await analyticsApi.getData(machineId, backendId, compareModel.value)
      if (res.success && res.data) {
        return res.data
      }
      return null
    })
    
    const results = await Promise.all(promises)
    compareData.value = results.filter((r): r is AnalyticsData => r !== null)
  } catch (e) {
    console.error('加载对比数据失败:', e)
  } finally {
    compareLoading.value = false
  }
}

// 移除选中的后端
function removeBackend(key: string) {
  selectedBackends.value = selectedBackends.value.filter(k => k !== key)
}

// 获取后端显示名称
function getBackendDisplayName(key: string): string {
  const parts = key.split('/')
  const machineId = parts[0] || ''
  const backendId = parts[1] || ''
  const machine = store.machines.find(m => m.id === machineId)
  const backends = machineId ? (store.machineBackends[machineId] || []) : []
  const backend = backends.find((b: Backend) => b.id === backendId)
  return `${machine?.name || machineId} / ${backend?.name || backendId}`
}

// 监听
watch(selectedHistoryId, () => {
  selectedModel.value = ''
  loadHistoryDetail()
})

watch(() => [store.currentMachineId, store.currentBackendId], () => {
  if (analysisMode.value === 'single') {
    selectedHistoryId.value = ''
    testResults.value = {}
    loadHistoryList()
  }
})

watch(analysisMode, (mode) => {
  if (mode === 'compare') {
    loadAllBackends()
    loadAvailableModels()
  }
})

watch([compareModel, selectedBackends], () => {
  if (analysisMode.value === 'compare') {
    loadCompareData()
  }
}, { deep: true })

// ========== 图表配置 ==========

// 单机模式：图表1 TPS+Prefill vs 并发数（双Y轴）
const singleTpsByConcurrencyOption = computed(() => {
  const results = filteredResults.value
  if (results.length === 0 || fixedContextForConcurrencyChart.value === null) return {}
  
  const targetContext = fixedContextForConcurrencyChart.value
  
  const data = results
    .filter(r => r.ContextTargetTokens === targetContext)
    .map(r => ({ 
      concurrency: r.ConcurrencyLevel, 
      tps: r.TokensPerSec,
      prefillTps: r.PrefillTokensPerSecAvg || 0,
      successRate: r.TotalRequests > 0 ? (r.SuccessRequests / r.TotalRequests * 100) : 0
    }))
    .sort((a, b) => a.concurrency - b.concurrency)
  
  if (data.length === 0) return {}
  
  return createDualAxisChart(
    `TPS & Prefill 随并发数变化（固定上下文 ${targetContext} tokens）`,
    data.map(d => d.concurrency),
    data.map(d => d.tps),
    data.map(d => d.prefillTps),
    '并发数',
    'Decode TPS',
    'Prefill TPS',
    '#6366f1',
    '#10b981',
    (params: any) => {
      if (!params || params.length === 0) return ''
      const idx = params[0].dataIndex
      const d = data[idx]
      if (!d) return ''
      return `<div style="padding: 8px">
        <div style="font-weight: bold; margin-bottom: 8px">并发数: ${d.concurrency}</div>
        <div style="color: #6366f1; margin: 4px 0">Decode TPS: ${d.tps.toFixed(2)}</div>
        <div style="color: #10b981; margin: 4px 0">Prefill TPS: ${d.prefillTps.toFixed(2)}</div>
        <div style="color: #a1a1aa; font-size: 12px; margin-top: 4px">成功率: ${d.successRate.toFixed(1)}%</div>
      </div>`
    }
  )
})

// 单机模式：图表2 TPS+Prefill vs 上下文长度（双Y轴）
const singleTpsByContextOption = computed(() => {
  const results = filteredResults.value
  if (results.length === 0 || fixedConcurrencyForContextChart.value === null) return {}
  
  const targetConcurrency = fixedConcurrencyForContextChart.value
  
  const data = results
    .filter(r => r.ConcurrencyLevel === targetConcurrency)
    .map(r => ({ 
      context: r.ContextTargetTokens, 
      tps: r.TokensPerSec,
      prefillTps: r.PrefillTokensPerSecAvg || 0,
      avgLatency: r.AvgLatency / 1e6
    }))
    .sort((a, b) => a.context - b.context)
  
  if (data.length === 0) return {}
  
  return createDualAxisChart(
    `TPS & Prefill 随上下文长度变化（固定 ${targetConcurrency} 并发）`,
    data.map(d => d.context),
    data.map(d => d.tps),
    data.map(d => d.prefillTps),
    '上下文长度 (tokens)',
    'Decode TPS',
    'Prefill TPS',
    '#6366f1',
    '#10b981',
    (params: any) => {
      if (!params || params.length === 0) return ''
      const idx = params[0].dataIndex
      const d = data[idx]
      if (!d) return ''
      return `<div style="padding: 8px">
        <div style="font-weight: bold; margin-bottom: 8px">上下文: ${d.context} tokens</div>
        <div style="color: #6366f1; margin: 4px 0">Decode TPS: ${d.tps.toFixed(2)}</div>
        <div style="color: #10b981; margin: 4px 0">Prefill TPS: ${d.prefillTps.toFixed(2)}</div>
        <div style="color: #a1a1aa; font-size: 12px; margin-top: 4px">平均延迟: ${d.avgLatency.toFixed(0)} ms</div>
      </div>`
    }
  )
})

// 对比模式：图表1 TPS vs 并发数（多曲线）
const compareTpsByConcurrencyOption = computed(() => {
  if (compareData.value.length === 0) return {}
  
  // 找出所有的上下文长度，取最大的
  const allContexts = new Set<number>()
  compareData.value.forEach(ad => {
    ad.data.forEach(d => allContexts.add(d.context_tokens))
  })
  const targetContext = Math.max(...Array.from(allContexts)) || 4096
  
  // 收集所有并发数
  const allConcurrencies = new Set<number>()
  compareData.value.forEach(ad => {
    ad.data.filter(d => d.context_tokens === targetContext).forEach(d => {
      allConcurrencies.add(d.concurrency)
    })
  })
  const xAxisData = Array.from(allConcurrencies).sort((a, b) => a - b)
  
  // 生成多系列
  const series = compareData.value.map((ad, index) => {
    const dataMap = new Map<number, number>()
    ad.data.filter(d => d.context_tokens === targetContext).forEach(d => {
      dataMap.set(d.concurrency, d.tps)
    })
    
    return {
      name: `${ad.machine_name}/${ad.backend_name}`,
      type: 'line' as const,
      data: xAxisData.map(c => dataMap.get(c) ?? null),
      smooth: true,
      symbol: 'circle',
      symbolSize: 6,
      lineStyle: { width: 2 },
      itemStyle: { color: chartColors[index % chartColors.length] }
    }
  })
  
  return createMultiLineChart(
    `TPS 随并发数变化（上下文 ${targetContext} tokens）`,
    xAxisData,
    series,
    '并发数',
    'TPS'
  )
})

// 对比模式：图表2 TPS vs 上下文长度（多曲线）
const compareTpsByContextOption = computed(() => {
  if (compareData.value.length === 0) return {}
  
  // 找出所有的并发数，优先取10
  const allConcurrencies = new Set<number>()
  compareData.value.forEach(ad => {
    ad.data.forEach(d => allConcurrencies.add(d.concurrency))
  })
  const concurrencyArr = Array.from(allConcurrencies).sort((a, b) => a - b)
  const targetConcurrency = concurrencyArr.includes(10) ? 10 : concurrencyArr[0] || 10
  
  // 收集所有上下文长度
  const allContexts = new Set<number>()
  compareData.value.forEach(ad => {
    ad.data.filter(d => d.concurrency === targetConcurrency).forEach(d => {
      allContexts.add(d.context_tokens)
    })
  })
  const xAxisData = Array.from(allContexts).sort((a, b) => a - b)
  
  // 生成多系列
  const series = compareData.value.map((ad, index) => {
    const dataMap = new Map<number, number>()
    ad.data.filter(d => d.concurrency === targetConcurrency).forEach(d => {
      dataMap.set(d.context_tokens, d.tps)
    })
    
    return {
      name: `${ad.machine_name}/${ad.backend_name}`,
      type: 'line' as const,
      data: xAxisData.map(c => dataMap.get(c) ?? null),
      smooth: true,
      symbol: 'circle',
      symbolSize: 6,
      lineStyle: { width: 2 },
      itemStyle: { color: chartColors[index % chartColors.length] }
    }
  })
  
  return createMultiLineChart(
    `TPS 随上下文长度变化（${targetConcurrency} 并发）`,
    xAxisData,
    series,
    '上下文长度 (tokens)',
    'TPS'
  )
})

// 创建双Y轴图表配置（TPS + Prefill）
function createDualAxisChart(
  title: string,
  xData: (string | number)[],
  tpsData: number[],
  prefillData: number[],
  xName: string,
  yLeftName: string,
  yRightName: string,
  tpsColor: string,
  prefillColor: string,
  formatter: (params: any) => string
) {
  return {
    backgroundColor: 'transparent',
    title: {
      text: title,
      left: 'center',
      textStyle: { color: '#fafafa', fontSize: 14 }
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(24, 24, 27, 0.9)',
      borderColor: '#3f3f46',
      textStyle: { color: '#fafafa' },
      formatter
    },
    legend: {
      top: 30,
      textStyle: { color: '#a1a1aa' },
      data: [yLeftName, yRightName]
    },
    grid: { left: '10%', right: '10%', top: '20%', bottom: '15%' },
    xAxis: {
      type: 'category',
      name: xName,
      nameTextStyle: { color: '#a1a1aa' },
      data: xData,
      axisLine: { lineStyle: { color: '#3f3f46' } },
      axisLabel: { color: '#a1a1aa' }
    },
    yAxis: [
      {
        type: 'value',
        name: yLeftName,
        position: 'left',
        nameTextStyle: { color: tpsColor },
        axisLine: { lineStyle: { color: tpsColor } },
        axisLabel: { color: tpsColor },
        splitLine: { lineStyle: { color: '#27272a' } }
      },
      {
        type: 'value',
        name: yRightName,
        position: 'right',
        nameTextStyle: { color: prefillColor },
        axisLine: { lineStyle: { color: prefillColor } },
        axisLabel: { color: prefillColor },
        splitLine: { show: false }
      }
    ],
    series: [
      {
        name: yLeftName,
        type: 'line',
        yAxisIndex: 0,
        data: tpsData,
        smooth: true,
        symbol: 'circle',
        symbolSize: 8,
        lineStyle: { color: tpsColor, width: 3 },
        itemStyle: { color: tpsColor, borderColor: '#fff', borderWidth: 2 },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(99, 102, 241, 0.3)' },
              { offset: 1, color: 'rgba(99, 102, 241, 0.05)' }
            ]
          }
        }
      },
      {
        name: yRightName,
        type: 'line',
        yAxisIndex: 1,
        data: prefillData,
        smooth: true,
        symbol: 'diamond',
        symbolSize: 8,
        lineStyle: { color: prefillColor, width: 3, type: 'dashed' },
        itemStyle: { color: prefillColor, borderColor: '#fff', borderWidth: 2 }
      }
    ]
  }
}

// 创建单曲线图表配置
function createSingleLineChart(
  title: string,
  xData: (string | number)[],
  yData: number[],
  xName: string,
  yName: string,
  color: string,
  formatter: (params: any) => string
) {
  return {
    backgroundColor: 'transparent',
    title: {
      text: title,
      left: 'center',
      textStyle: { color: '#fafafa', fontSize: 14 }
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(24, 24, 27, 0.9)',
      borderColor: '#3f3f46',
      textStyle: { color: '#fafafa' },
      formatter
    },
    grid: { left: '10%', right: '5%', top: '15%', bottom: '15%' },
    xAxis: {
      type: 'category',
      name: xName,
      nameTextStyle: { color: '#a1a1aa' },
      data: xData,
      axisLine: { lineStyle: { color: '#3f3f46' } },
      axisLabel: { color: '#a1a1aa' }
    },
    yAxis: {
      type: 'value',
      name: yName,
      nameTextStyle: { color: '#a1a1aa' },
      axisLine: { lineStyle: { color: '#3f3f46' } },
      axisLabel: { color: '#a1a1aa' },
      splitLine: { lineStyle: { color: '#27272a' } }
    },
    series: [{
      type: 'line',
      data: yData,
      smooth: true,
      symbol: 'circle',
      symbolSize: 8,
      lineStyle: { color, width: 3 },
      itemStyle: { color, borderColor: '#fff', borderWidth: 2 },
      areaStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: color.replace(')', ', 0.3)').replace('rgb', 'rgba') },
            { offset: 1, color: color.replace(')', ', 0.05)').replace('rgb', 'rgba') }
          ]
        }
      }
    }]
  }
}

// 创建多曲线图表配置
function createMultiLineChart(
  title: string,
  xData: (string | number)[],
  series: any[],
  xName: string,
  yName: string
) {
  return {
    backgroundColor: 'transparent',
    title: {
      text: title,
      left: 'center',
      textStyle: { color: '#fafafa', fontSize: 14 }
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(24, 24, 27, 0.9)',
      borderColor: '#3f3f46',
      textStyle: { color: '#fafafa' }
    },
    legend: {
      top: 30,
      textStyle: { color: '#a1a1aa' },
      icon: 'roundRect'
    },
    grid: { left: '10%', right: '5%', top: '20%', bottom: '15%' },
    xAxis: {
      type: 'category',
      name: xName,
      nameTextStyle: { color: '#a1a1aa' },
      data: xData,
      axisLine: { lineStyle: { color: '#3f3f46' } },
      axisLabel: { color: '#a1a1aa' }
    },
    yAxis: {
      type: 'value',
      name: yName,
      nameTextStyle: { color: '#a1a1aa' },
      axisLine: { lineStyle: { color: '#3f3f46' } },
      axisLabel: { color: '#a1a1aa' },
      splitLine: { lineStyle: { color: '#27272a' } }
    },
    series
  }
}

// 是否有数据
const hasSingleData = computed(() => {
  return Object.keys(singleTpsByConcurrencyOption.value).length > 0 || 
         Object.keys(singleTpsByContextOption.value).length > 0
})

const hasCompareData = computed(() => {
  return Object.keys(compareTpsByConcurrencyOption.value).length > 0 || 
         Object.keys(compareTpsByContextOption.value).length > 0
})

onMounted(() => {
  loadHistoryList()
})
</script>

<template>
  <div class="analytics-panel">
    <div class="panel-header">
      <div class="header-left">
        <NIcon size="24" color="#6366f1">
          <AnalyticsOutline />
        </NIcon>
        <h3>数据分析</h3>
      </div>
      
      <NSpace align="center">
        <NRadioGroup v-model:value="analysisMode" size="small">
          <NRadioButton value="single">单机分析</NRadioButton>
          <NRadioButton value="compare">对比分析</NRadioButton>
        </NRadioGroup>
      </NSpace>
    </div>
    
    <!-- 单机分析模式 -->
    <template v-if="analysisMode === 'single'">
      <div class="filter-bar">
        <NSpace>
          <NSelect
            v-model:value="selectedHistoryId"
            :options="historyOptions"
            style="width: 300px"
            size="small"
            placeholder="选择历史记录"
            :disabled="historyOptions.length === 0"
          />
          <NSelect
            v-model:value="selectedModel"
            :options="modelOptions"
            style="width: 200px"
            size="small"
            placeholder="选择模型"
            :disabled="modelOptions.length === 0"
          />
        </NSpace>
      </div>
      
      <!-- 固定维度筛选 -->
      <div class="filter-bar" v-if="selectedModel && filteredResults.length > 0">
        <NSpace align="center">
          <span class="filter-label">图表1固定上下文:</span>
          <NSelect
            v-model:value="fixedContextForConcurrencyChart"
            :options="availableContextLengths"
            style="width: 160px"
            size="small"
            placeholder="选择上下文"
          />
          <span class="filter-label" style="margin-left: 16px">图表2固定并发:</span>
          <NSelect
            v-model:value="fixedConcurrencyForContextChart"
            :options="availableConcurrencies"
            style="width: 140px"
            size="small"
            placeholder="选择并发数"
          />
        </NSpace>
      </div>
      
      <NAlert type="info" :bordered="false" style="margin-bottom: 16px">
        从当前后端的历史测试结果中分析 TPS 和 Prefill 速度随并发数和上下文长度的变化趋势。图表1固定上下文长度观察并发影响，图表2固定并发数观察上下文长度影响。
      </NAlert>
      
      <NSpin :show="loading">
        <template v-if="hasSingleData && selectedModel">
          <NGrid :cols="1" :y-gap="16">
            <NGi v-if="Object.keys(singleTpsByConcurrencyOption).length > 0">
              <NCard class="chart-card">
                <VChart :option="singleTpsByConcurrencyOption" autoresize style="height: 350px" />
              </NCard>
            </NGi>
            <NGi v-if="Object.keys(singleTpsByContextOption).length > 0">
              <NCard class="chart-card">
                <VChart :option="singleTpsByContextOption" autoresize style="height: 350px" />
              </NCard>
            </NGi>
          </NGrid>
        </template>
        <NEmpty v-else description="请选择历史记录和模型查看分析数据" style="padding: 60px 0">
          <template #icon>
            <NIcon size="48" color="#3f3f46"><AnalyticsOutline /></NIcon>
          </template>
        </NEmpty>
      </NSpin>
    </template>
    
    <!-- 对比分析模式 -->
    <template v-else>
      <div class="filter-bar">
        <NSpace align="center">
          <NSelect
            v-model:value="compareModel"
            :options="availableModels.map(m => ({ label: m, value: m }))"
            style="width: 200px"
            size="small"
            placeholder="选择模型"
            :disabled="availableModels.length === 0"
          />
          <NCascader
            v-model:value="selectedBackends"
            :options="backendCascaderOptions"
            multiple
            check-strategy="child"
            style="width: 350px"
            size="small"
            placeholder="选择要对比的机器/后端"
            clearable
            max-tag-count="responsive"
          />
        </NSpace>
      </div>
      
      <!-- 已选择的后端标签 -->
      <div class="selected-tags" v-if="selectedBackends.length > 0">
        <span class="tags-label">已选择：</span>
        <NTag 
          v-for="key in selectedBackends" 
          :key="key"
          closable
          @close="removeBackend(key)"
          size="small"
          :color="{ color: '#27272a', textColor: '#fafafa', borderColor: '#3f3f46' }"
        >
          {{ getBackendDisplayName(key) }}
        </NTag>
      </div>
      
      <NAlert type="info" :bordered="false" style="margin-bottom: 16px">
        选择同一模型，对比不同机器/后端的性能表现。支持多选进行多曲线对比。
      </NAlert>
      
      <NSpin :show="compareLoading">
        <template v-if="hasCompareData">
          <NGrid :cols="1" :y-gap="16">
            <NGi v-if="Object.keys(compareTpsByConcurrencyOption).length > 0">
              <NCard class="chart-card">
                <VChart :option="compareTpsByConcurrencyOption" autoresize style="height: 400px" />
              </NCard>
            </NGi>
            <NGi v-if="Object.keys(compareTpsByContextOption).length > 0">
              <NCard class="chart-card">
                <VChart :option="compareTpsByContextOption" autoresize style="height: 400px" />
              </NCard>
            </NGi>
          </NGrid>
        </template>
        <NEmpty v-else description="请选择模型和要对比的机器/后端" style="padding: 60px 0">
          <template #icon>
            <NIcon size="48" color="#3f3f46"><AnalyticsOutline /></NIcon>
          </template>
        </NEmpty>
      </NSpin>
    </template>
  </div>
</template>

<style scoped>
.analytics-panel {
  padding: 0;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #fafafa;
}

.filter-bar {
  margin-bottom: 16px;
}

.filter-label {
  color: #a1a1aa;
  font-size: 13px;
  white-space: nowrap;
}

.selected-tags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
  padding: 12px;
  background: #1f1f23;
  border-radius: 8px;
}

.tags-label {
  color: #a1a1aa;
  font-size: 13px;
  margin-right: 4px;
}

.chart-card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
}

.chart-card :deep(.n-card__content) {
  padding: 16px;
}
</style>
