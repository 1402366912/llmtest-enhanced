<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { 
  NCard, NGrid, NGi, NDataTable, NEmpty, NTabs, NTabPane, NTag, NButton, NIcon
} from 'naive-ui'
import { DownloadOutline, RefreshOutline } from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { 
  TitleComponent, TooltipComponent, LegendComponent, GridComponent 
} from 'echarts/components'
import type { TestResult } from '@/types'

use([CanvasRenderer, BarChart, LineChart, TitleComponent, TooltipComponent, LegendComponent, GridComponent])

const store = useAppStore()

// 结果数据
const results = computed(() => store.testResults)

// 表格列
const columns = [
  { title: '模型', key: 'ModelName', width: 150, fixed: 'left' as const },
  { title: '并发度', key: 'ConcurrencyLevel', width: 80 },
  { title: '上下文Token', key: 'ContextTargetTokens', width: 100 },
  { title: '成功/总请求', key: 'requests', width: 100, 
    render: (row: TestResult) => `${row.SuccessRequests}/${row.TotalRequests}` },
  { title: '成功率', key: 'successRate', width: 80,
    render: (row: TestResult) => {
      const rate = row.TotalRequests > 0 ? (row.SuccessRequests / row.TotalRequests * 100) : 0
      return h(NTag, { 
        type: rate >= 99 ? 'success' : rate >= 90 ? 'warning' : 'error',
        size: 'small'
      }, { default: () => rate.toFixed(1) + '%' })
    }
  },
  { title: 'RPS', key: 'RequestsPerSec', width: 80,
    render: (row: TestResult) => row.RequestsPerSec?.toFixed(2) || '-' },
  { title: 'TPS', key: 'TokensPerSec', width: 100,
    render: (row: TestResult) => h('span', { class: 'metric-highlight' }, row.TokensPerSec?.toFixed(2) || '-') },
  { title: 'Prefill TPS', key: 'PrefillTokensPerSecAvg', width: 100,
    render: (row: TestResult) => row.PrefillTokensPerSecAvg?.toFixed(2) || '-' },
  { title: 'Decode TPS', key: 'DecodeTokensPerSecAvg', width: 100,
    render: (row: TestResult) => row.DecodeTokensPerSecAvg?.toFixed(2) || '-' },
  { title: '平均延迟(ms)', key: 'AvgLatency', width: 100,
    render: (row: TestResult) => (row.AvgLatency / 1000000)?.toFixed(2) || '-' },
  { title: 'TTFT均值(ms)', key: 'AvgFirstTokenLatency', width: 110,
    render: (row: TestResult) => (row.AvgFirstTokenLatency / 1000000)?.toFixed(2) || '-' },
  { title: 'TTFT P50(ms)', key: 'TTFTP50', width: 110,
    render: (row: TestResult) => row.FirstTokenPercentiles?.[50]
      ? (row.FirstTokenPercentiles[50] / 1000000).toFixed(2)
      : '-' },
  { title: 'TTFT P90(ms)', key: 'TTFTP90', width: 110,
    render: (row: TestResult) => row.FirstTokenPercentiles?.[90]
      ? (row.FirstTokenPercentiles[90] / 1000000).toFixed(2)
      : '-' },
  { title: 'TTFT P99(ms)', key: 'TTFTP99', width: 110,
    render: (row: TestResult) => row.FirstTokenPercentiles?.[99]
      ? (row.FirstTokenPercentiles[99] / 1000000).toFixed(2)
      : '-' },
  { title: '平均输入Token', key: 'AvgInputTokens', width: 100,
    render: (row: TestResult) => row.AvgInputTokens?.toFixed(0) || '-' },
  { title: '平均输出Token', key: 'AvgOutputTokens', width: 100,
    render: (row: TestResult) => row.AvgOutputTokens?.toFixed(0) || '-' },
]

// 表格数据
const tableData = computed(() => {
  if (!results.value) return []
  return Object.values(results.value)
})

// TPS 柱状图配置
const tpsChartOption = computed(() => {
  if (!results.value) return {}
  
  const data = Object.values(results.value)
  const labels = data.map(r => `${r.ModelName}\n(${r.ConcurrencyLevel}并发)`)
  const tpsValues = data.map(r => r.TokensPerSec || 0)
  
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: labels,
      axisLabel: {
        color: '#a1a1aa',
        interval: 0,
        rotate: 30
      },
      axisLine: { lineStyle: { color: '#3f3f46' } }
    },
    yAxis: {
      type: 'value',
      name: 'TPS',
      nameTextStyle: { color: '#a1a1aa' },
      axisLabel: { color: '#a1a1aa' },
      axisLine: { lineStyle: { color: '#3f3f46' } },
      splitLine: { lineStyle: { color: '#27272a' } }
    },
    series: [{
      type: 'bar',
      data: tpsValues,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: '#818cf8' },
            { offset: 1, color: '#6366f1' }
          ]
        },
        borderRadius: [4, 4, 0, 0]
      },
      barMaxWidth: 50
    }]
  }
})

// 延迟分布图配置
const latencyChartOption = computed(() => {
  if (!results.value) return {}
  
  const data = Object.values(results.value)
  const labels = data.map(r => `${r.ModelName}\n(${r.ConcurrencyLevel}并发)`)
  
  const p50 = data.map(r => r.LatencyPercentiles?.[50] ? r.LatencyPercentiles[50] / 1000000 : 0)
  const p90 = data.map(r => r.LatencyPercentiles?.[90] ? r.LatencyPercentiles[90] / 1000000 : 0)
  const p99 = data.map(r => r.LatencyPercentiles?.[99] ? r.LatencyPercentiles[99] / 1000000 : 0)
  
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis'
    },
    legend: {
      data: ['P50', 'P90', 'P99'],
      textStyle: { color: '#a1a1aa' },
      top: 0
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      top: '15%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: labels,
      axisLabel: {
        color: '#a1a1aa',
        interval: 0,
        rotate: 30
      },
      axisLine: { lineStyle: { color: '#3f3f46' } }
    },
    yAxis: {
      type: 'value',
      name: '延迟 (ms)',
      nameTextStyle: { color: '#a1a1aa' },
      axisLabel: { color: '#a1a1aa' },
      axisLine: { lineStyle: { color: '#3f3f46' } },
      splitLine: { lineStyle: { color: '#27272a' } }
    },
    series: [
      {
        name: 'P50',
        type: 'bar',
        data: p50,
        itemStyle: { color: '#10b981' },
        barMaxWidth: 30
      },
      {
        name: 'P90',
        type: 'bar',
        data: p90,
        itemStyle: { color: '#f59e0b' },
        barMaxWidth: 30
      },
      {
        name: 'P99',
        type: 'bar',
        data: p99,
        itemStyle: { color: '#ef4444' },
        barMaxWidth: 30
      }
    ]
  }
})

// 刷新结果
async function refreshResults() {
  await store.loadTestResults()
}

import { h } from 'vue'
</script>

<template>
  <div class="results-panel">
    <div class="panel-header">
      <h3>测试结果</h3>
      <NButton quaternary size="small" @click="refreshResults">
        <template #icon><NIcon><RefreshOutline /></NIcon></template>
        刷新
      </NButton>
    </div>
    
    <template v-if="results && Object.keys(results).length > 0">
      <!-- 概览指标卡片 -->
      <NGrid :cols="4" :x-gap="16" :y-gap="16" class="summary-cards">
        <NGi>
          <div class="summary-card">
            <div class="summary-value">{{ Object.keys(results).length }}</div>
            <div class="summary-label">测试组数</div>
          </div>
        </NGi>
        <NGi>
          <div class="summary-card">
            <div class="summary-value">
              {{ Math.max(...Object.values(results).map(r => r.TokensPerSec || 0)).toFixed(1) }}
            </div>
            <div class="summary-label">最高 TPS</div>
          </div>
        </NGi>
        <NGi>
          <div class="summary-card">
            <div class="summary-value">
              {{ Math.max(...Object.values(results).map(r => r.RequestsPerSec || 0)).toFixed(2) }}
            </div>
            <div class="summary-label">最高 RPS</div>
          </div>
        </NGi>
        <NGi>
          <div class="summary-card">
            <div class="summary-value">
              {{ (Object.values(results).reduce((sum, r) => sum + r.SuccessRequests, 0) / 
                  Object.values(results).reduce((sum, r) => sum + r.TotalRequests, 0) * 100).toFixed(1) }}%
            </div>
            <div class="summary-label">平均成功率</div>
          </div>
        </NGi>
      </NGrid>
      
      <NTabs type="line" animated>
        <!-- 图表视图 -->
        <NTabPane name="charts" tab="图表">
          <NGrid :cols="2" :x-gap="16" :y-gap="16">
            <NGi>
              <NCard title="TPS 性能对比">
                <VChart :option="tpsChartOption" autoresize style="height: 300px" />
              </NCard>
            </NGi>
            <NGi>
              <NCard title="延迟分布">
                <VChart :option="latencyChartOption" autoresize style="height: 300px" />
              </NCard>
            </NGi>
          </NGrid>
        </NTabPane>
        
        <!-- 表格视图 -->
        <NTabPane name="table" tab="详细数据">
          <NCard>
            <NDataTable
              :columns="columns"
              :data="tableData"
              :bordered="false"
              :single-line="false"
              size="small"
              :scroll-x="1400"
              max-height="500"
            />
          </NCard>
        </NTabPane>
      </NTabs>
    </template>
    
    <NEmpty v-else description="暂无测试结果" style="padding: 60px 0">
      <template #extra>
        <p style="color: var(--text-muted)">执行测试后将在此显示结果</p>
      </template>
    </NEmpty>
  </div>
</template>

<style scoped>
.results-panel {
  max-width: 1200px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.panel-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 500;
}

.summary-cards {
  margin-bottom: 20px;
}

.summary-card {
  padding: 20px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  text-align: center;
}

.summary-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 28px;
  font-weight: 600;
  background: var(--gradient-primary);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.summary-label {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

:deep(.metric-highlight) {
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
  color: #818cf8;
}

:deep(.n-card) {
  margin-bottom: 16px;
}
</style>

