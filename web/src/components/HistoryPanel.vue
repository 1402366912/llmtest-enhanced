<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { 
  NCard, NDataTable, NButton, NSpace, NIcon, NEmpty, NTag, NPopconfirm, NModal,
  useMessage
} from 'naive-ui'
import { 
  DownloadOutline, TrashOutline, RefreshOutline, EyeOutline 
} from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import { historyApi } from '@/api'
import type { HistoryRecord, TestResult } from '@/types'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TitleComponent, TooltipComponent, GridComponent } from 'echarts/components'

use([CanvasRenderer, BarChart, TitleComponent, TooltipComponent, GridComponent])

const store = useAppStore()
const message = useMessage()

// 详情对话框
const showDetailModal = ref(false)
const detailRecord = ref<HistoryRecord | null>(null)
const detailResults = ref<Record<string, TestResult> | null>(null)
const loadingDetail = ref(false)

// 表格列
const columns = [
  { 
    title: '测试时间', 
    key: 'created_at', 
    width: 180,
    render: (row: HistoryRecord) => formatDate(row.created_at)
  },
  { 
    title: '文件名', 
    key: 'file_name',
    ellipsis: { tooltip: true }
  },
  { 
    title: '模型数', 
    key: 'model_count', 
    width: 80,
    render: (row: HistoryRecord) => row.summary?.model_count || '-'
  },
  { 
    title: '总请求', 
    key: 'total_requests', 
    width: 100,
    render: (row: HistoryRecord) => row.summary?.total_requests || '-'
  },
  { 
    title: '最高TPS', 
    key: 'max_tps', 
    width: 100,
    render: (row: HistoryRecord) => row.summary?.max_tps?.toFixed(2) || '-'
  },
  { 
    title: '最高RPS', 
    key: 'max_rps', 
    width: 100,
    render: (row: HistoryRecord) => row.summary?.max_rps?.toFixed(2) || '-'
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row: HistoryRecord) => h(NSpace, null, {
      default: () => [
        h(NButton, { 
          size: 'small', 
          quaternary: true,
          onClick: () => viewDetail(row)
        }, { 
          icon: () => h(NIcon, null, { default: () => h(EyeOutline) }),
          default: () => '查看'
        }),
        h(NButton, { 
          size: 'small', 
          quaternary: true,
          tag: 'a',
          href: historyApi.exportUrl(row.id, row.machine_id, row.backend_id),
          target: '_blank'
        }, { 
          icon: () => h(NIcon, null, { default: () => h(DownloadOutline) }),
          default: () => '下载'
        }),
        h(NPopconfirm, {
          onPositiveClick: () => deleteRecord(row)
        }, {
          trigger: () => h(NButton, { 
            size: 'small', 
            quaternary: true,
            type: 'error'
          }, { 
            icon: () => h(NIcon, null, { default: () => h(TrashOutline) }),
            default: () => '删除'
          }),
          default: () => '确定删除该记录?'
        })
      ]
    })
  }
]

// 格式化日期
function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

// 加载历史记录
async function loadHistory() {
  await store.loadHistory()
}

// 查看详情
async function viewDetail(record: HistoryRecord) {
  detailRecord.value = record
  loadingDetail.value = true
  showDetailModal.value = true
  
  try {
    const res = await historyApi.getDetail(record.id, record.machine_id, record.backend_id)
    if (res.success && res.data) {
      detailResults.value = res.data
    }
  } catch (e: any) {
    message.error('加载详情失败')
  } finally {
    loadingDetail.value = false
  }
}

// 删除记录
async function deleteRecord(record: HistoryRecord) {
  try {
    await store.deleteHistory(record.id, record.machine_id, record.backend_id)
    message.success('已删除')
  } catch (e: any) {
    message.error(e.message || '删除失败')
  }
}

// 格式化上下文长度显示
function formatContextTokens(tokens: number): string {
  if (!tokens || tokens === 0) return '无上下文'
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(0)}K`
  return `${tokens}`
}

// 详情图表配置
const detailChartOption = computed(() => {
  if (!detailResults.value) return {}
  
  const data = Object.values(detailResults.value)
  // 显示模型名、并发数、上下文长度
  const labels = data.map(r => {
    const ctx = formatContextTokens(r.ContextTargetTokens || 0)
    return `${r.ModelName}\n${r.ConcurrencyLevel}并发/${ctx}`
  })
  const tpsValues = data.map(r => r.TokensPerSec || 0)
  
  return {
    backgroundColor: 'transparent',
    tooltip: { 
      trigger: 'axis',
      formatter: (params: any) => {
        const p = params[0]
        const result = data[p.dataIndex]
        if (!result) return ''
        const ctx = formatContextTokens(result.ContextTargetTokens || 0)
        return `<div style="font-weight: 500">${result.ModelName}</div>
          <div>并发: ${result.ConcurrencyLevel}</div>
          <div>上下文: ${ctx}</div>
          <div style="margin-top: 4px">TPS: <b>${(result.TokensPerSec || 0).toFixed(2)}</b></div>`
      }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '20%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: labels,
      axisLabel: { 
        color: '#a1a1aa', 
        interval: 0, 
        rotate: 30,
        fontSize: 11
      },
      axisLine: { lineStyle: { color: '#3f3f46' } }
    },
    yAxis: {
      type: 'value',
      name: 'TPS',
      nameTextStyle: { color: '#a1a1aa', padding: [0, 0, 0, 40] },
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

onMounted(() => {
  loadHistory()
})

import { h } from 'vue'
</script>

<template>
  <div class="history-panel">
    <div class="panel-header">
      <h3>历史记录</h3>
      <NButton quaternary size="small" @click="loadHistory">
        <template #icon><NIcon><RefreshOutline /></NIcon></template>
        刷新
      </NButton>
    </div>
    
    <NCard>
      <NDataTable
        v-if="store.historyRecords.length > 0"
        :columns="columns"
        :data="store.historyRecords"
        :bordered="false"
        size="small"
        :pagination="{ pageSize: 10 }"
      />
      
      <NEmpty v-else description="暂无历史记录" style="padding: 40px 0">
        <template #extra>
          <p style="color: var(--text-muted)">执行测试后将自动保存历史记录</p>
        </template>
      </NEmpty>
    </NCard>
    
    <!-- 详情对话框 -->
    <NModal
      v-model:show="showDetailModal"
      preset="card"
      :title="detailRecord?.file_name || '测试详情'"
      style="width: 800px"
    >
      <template v-if="!loadingDetail && detailResults">
        <VChart :option="detailChartOption" autoresize style="height: 300px" />
        
        <div class="detail-stats">
          <div class="stat-item" v-for="(result, key) in detailResults" :key="key">
            <div class="stat-header">
              <span class="model-name">{{ result.ModelName }}</span>
              <NTag size="small">并发 {{ result.ConcurrencyLevel }}</NTag>
              <NTag size="small" type="info">
                {{ result.ContextTargetTokens ? `上下文 ${formatContextTokens(result.ContextTargetTokens)}` : '无上下文' }}
              </NTag>
            </div>
            <div class="stat-values">
              <span>TPS: <strong>{{ result.TokensPerSec?.toFixed(2) }}</strong></span>
              <span>RPS: <strong>{{ result.RequestsPerSec?.toFixed(2) }}</strong></span>
              <span>成功率: <strong>{{ (result.SuccessRequests / result.TotalRequests * 100).toFixed(1) }}%</strong></span>
            </div>
          </div>
        </div>
      </template>
      
      <div v-else-if="loadingDetail" style="text-align: center; padding: 40px">
        加载中...
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.history-panel {
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

.detail-stats {
  margin-top: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.stat-item {
  padding: 12px 16px;
  background: var(--bg-elevated);
  border-radius: 8px;
}

.stat-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.model-name {
  font-weight: 500;
}

.stat-values {
  display: flex;
  gap: 24px;
  font-size: 14px;
  color: var(--text-secondary);
}

.stat-values strong {
  color: var(--text-primary);
  font-family: 'JetBrains Mono', monospace;
}
</style>

