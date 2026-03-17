<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { 
  NCard, NSelect, NSpace, NEmpty, NIcon, NTag, NAlert
} from 'naive-ui'
import { TrophyOutline } from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TitleComponent, TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'

use([CanvasRenderer, BarChart, TitleComponent, TooltipComponent, GridComponent, LegendComponent])

const store = useAppStore()

// 筛选条件
const selectedModel = ref<string | null>(null)
const selectedConcurrency = ref<number | null>(null)
const selectedContextTokens = ref<number | null>(null)

// 模型选项
const modelOptions = computed(() => {
  return store.leaderboardModels.map(m => ({ label: m, value: m }))
})

// 并发数选项
const concurrencyOptions = computed(() => {
  return store.leaderboardConcurrencies.map(c => ({ label: `${c} 并发`, value: c }))
})

// 上下文长度选项
const contextTokensOptions = computed(() => {
  return store.leaderboardContextTokens.map(c => {
    const label = c >= 1024 ? `${Math.round(c / 1024)}K` : `${c}`
    return { label: `上下文 ${label}`, value: c }
  })
})

// 加载数据
async function loadLeaderboard() {
  await store.loadLeaderboard({
    metric: 'tps',
    model: selectedModel.value || undefined,
    concurrency: selectedConcurrency.value || undefined,
    context_tokens: selectedContextTokens.value || undefined
  })
}

// 监听筛选条件变化
watch([selectedModel, selectedConcurrency, selectedContextTokens], () => {
  loadLeaderboard()
})

// 格式化上下文长度显示
function formatContext(tokens: number): string {
  if (tokens >= 1024) {
    return `${Math.round(tokens / 1024)}K`
  }
  return `${tokens}`
}

// 当前筛选条件描述
const filterDescription = computed(() => {
  const parts: string[] = []
  if (selectedModel.value) {
    parts.push(`模型: ${selectedModel.value}`)
  }
  if (selectedConcurrency.value) {
    parts.push(`${selectedConcurrency.value} 并发`)
  }
  if (selectedContextTokens.value) {
    parts.push(`上下文 ${formatContext(selectedContextTokens.value)}`)
  }
  return parts.length > 0 ? parts.join('，') : '全部数据'
})

// 图表配置
const chartOption = computed(() => {
  const data = store.leaderboard
  if (data.length === 0) return {}
  
  // 生成标签：机器名 / 后端名
  const labels = data.map(d => `${d.machine_name}\n${d.backend_name}`)
  const tpsValues = data.map(d => d.tps)
  
  // 生成渐变颜色
  const colors = ['#10b981', '#34d399', '#6ee7b7']
  const barColors = data.map((_, i) => {
    const ratio = i / Math.max(data.length - 1, 1)
    return {
      type: 'linear',
      x: 0, y: 0, x2: 1, y2: 0,
      colorStops: [
        { offset: 0, color: colors[0] },
        { offset: 1, color: colors[Math.min(Math.floor(ratio * 2), 2)] }
      ]
    }
  })
  
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: any) => {
        const item = params[0]
        const entry = data[item.dataIndex]
        if (!entry) return ''
        return `
          <div style="padding: 8px">
            <div style="font-weight: bold; margin-bottom: 8px">#${entry.rank} ${entry.machine_name} / ${entry.backend_name}</div>
            <div>GPU: ${entry.gpu_model || '-'}</div>
            <div>模型: ${entry.model_name}</div>
            <div>并发: ${entry.concurrency} | 上下文: ${formatContext(entry.context_tokens)}</div>
            <div style="margin-top: 8px; font-size: 16px; font-weight: bold">
              TPS: ${item.value.toFixed(2)}
            </div>
            <div>成功率: ${entry.success_rate.toFixed(1)}%</div>
          </div>
        `
      }
    },
    grid: {
      left: '3%',
      right: '15%',
      bottom: '3%',
      top: '3%',
      containLabel: true
    },
    xAxis: {
      type: 'value',
      name: 'TPS',
      nameTextStyle: { color: '#a1a1aa' },
      axisLabel: { color: '#a1a1aa' },
      axisLine: { lineStyle: { color: '#3f3f46' } },
      splitLine: { lineStyle: { color: '#27272a' } }
    },
    yAxis: {
      type: 'category',
      data: labels,
      inverse: true,
      axisLabel: { 
        color: '#a1a1aa',
        width: 150,
        overflow: 'truncate'
      },
      axisLine: { lineStyle: { color: '#3f3f46' } }
    },
    series: [{
      type: 'bar',
      data: tpsValues.map((v, i) => ({
        value: v,
        itemStyle: { color: barColors[i], borderRadius: [0, 4, 4, 0] }
      })),
      barMaxWidth: 30,
      label: {
        show: true,
        position: 'right',
        color: '#fafafa',
        formatter: (params: any) => {
          const entry = data[params.dataIndex]
          if (!entry) return params.value.toFixed(2)
          return `#${entry.rank} ${params.value.toFixed(2)}`
        }
      }
    }]
  }
})

// 计算图表高度
const chartHeight = computed(() => {
  return Math.max(400, store.leaderboard.length * 60)
})

onMounted(() => {
  loadLeaderboard()
})
</script>

<template>
  <div class="leaderboard-panel">
    <div class="panel-header">
      <div class="header-left">
        <NIcon size="24" color="#f59e0b">
          <TrophyOutline />
        </NIcon>
        <h3>性能天梯图</h3>
      </div>
      
      <NSpace>
        <NSelect
          v-model:value="selectedModel"
          :options="modelOptions"
          style="width: 280px"
          size="small"
          placeholder="选择模型"
          clearable
        />
        <NSelect
          v-model:value="selectedConcurrency"
          :options="concurrencyOptions"
          style="width: 120px"
          size="small"
          placeholder="并发数"
          clearable
        />
        <NSelect
          v-model:value="selectedContextTokens"
          :options="contextTokensOptions"
          style="width: 140px"
          size="small"
          placeholder="上下文"
          clearable
        />
      </NSpace>
    </div>
    
    <!-- 筛选条件说明 -->
    <NAlert type="info" :bordered="false" style="margin-bottom: 16px">
      <template #header>
        天梯图筛选条件
      </template>
      当前显示：<strong>{{ filterDescription }}</strong>，按 TPS 从高到低排序。
      <template v-if="store.leaderboardModels.length === 0">
        <br/>暂无测试数据，请先进行测试。
      </template>
    </NAlert>
    
    <NCard v-if="store.leaderboard.length > 0">
      <VChart 
        :option="chartOption" 
        autoresize 
        :style="{ height: chartHeight + 'px' }"
      />
      
      <!-- 排行榜列表 -->
      <div class="ranking-list">
        <div 
          v-for="entry in store.leaderboard" 
          :key="`${entry.machine_id}-${entry.backend_id}-${entry.concurrency}-${entry.context_tokens}`"
          class="ranking-item"
          :class="{ 'top-3': entry.rank <= 3 }"
        >
          <div class="rank-badge" :class="`rank-${entry.rank}`">
            {{ entry.rank }}
          </div>
          <div class="entry-info">
            <div class="entry-main">
              <span class="machine-name">{{ entry.machine_name }}</span>
              <span class="separator">/</span>
              <span class="backend-name">{{ entry.backend_name }}</span>
              <NTag size="small" v-if="entry.gpu_model">{{ entry.gpu_model }}</NTag>
            </div>
            <div class="entry-sub">
              {{ entry.model_name }} · {{ entry.concurrency }}并发 · {{ formatContext(entry.context_tokens) }}ctx
            </div>
          </div>
          <div class="entry-metrics">
            <div class="metric primary">
              <span class="metric-value">{{ entry.tps.toFixed(2) }}</span>
              <span class="metric-label">TPS</span>
            </div>
            <div class="metric">
              <span class="metric-value">{{ entry.avg_latency_ms.toFixed(0) }}</span>
              <span class="metric-label">延迟ms</span>
            </div>
            <div class="metric">
              <span class="metric-value">{{ entry.success_rate.toFixed(0) }}%</span>
              <span class="metric-label">成功率</span>
            </div>
          </div>
        </div>
      </div>
    </NCard>
    
    <NEmpty v-else-if="store.leaderboardModels.length > 0" description="暂无符合条件的天梯图数据" style="padding: 60px 0">
      <template #extra>
        <p style="color: var(--text-muted)">
          请调整筛选条件或进行更多测试
        </p>
      </template>
    </NEmpty>
    
    <NEmpty v-else description="暂无测试数据" style="padding: 60px 0">
      <template #extra>
        <p style="color: var(--text-muted)">请先进行测试，然后在此查看性能对比</p>
      </template>
    </NEmpty>
  </div>
</template>

<style scoped>
.leaderboard-panel {
  max-width: 1200px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  flex-wrap: wrap;
  gap: 12px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-left h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 500;
}

.ranking-list {
  margin-top: 24px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ranking-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 16px;
  background: var(--bg-elevated);
  border-radius: 8px;
  transition: background 0.2s;
}

.ranking-item:hover {
  background: var(--bg-card-hover);
}

.ranking-item.top-3 {
  background: linear-gradient(135deg, rgba(245, 158, 11, 0.1) 0%, rgba(16, 185, 129, 0.1) 100%);
}

.rank-badge {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font-weight: 600;
  font-size: 14px;
  background: var(--bg-card);
  color: var(--text-secondary);
}

.rank-badge.rank-1 {
  background: linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%);
  color: #18181b;
}

.rank-badge.rank-2 {
  background: linear-gradient(135deg, #9ca3af 0%, #d1d5db 100%);
  color: #18181b;
}

.rank-badge.rank-3 {
  background: linear-gradient(135deg, #b45309 0%, #d97706 100%);
  color: white;
}

.entry-info {
  flex: 1;
}

.entry-main {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.machine-name {
  font-weight: 600;
  color: var(--text-primary);
}

.separator {
  color: var(--text-muted);
}

.backend-name {
  font-weight: 500;
  color: #10b981;
}

.entry-sub {
  font-size: 13px;
  color: var(--text-muted);
}

.entry-metrics {
  display: flex;
  gap: 24px;
}

.metric {
  text-align: right;
}

.metric.primary .metric-value {
  color: #10b981;
  font-size: 18px;
}

.metric-value {
  display: block;
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
  font-size: 16px;
}

.metric-label {
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
}
</style>
