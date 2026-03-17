<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { 
  NCard, NButton, NSpace, NProgress, NStatistic, NGrid, NGi, NIcon, NTag, NAlert, NSpin, NSelect, NDescriptions, NDescriptionsItem
} from 'naive-ui'
import { PlayOutline, StopOutline, RefreshOutline, FlashOutline, CheckmarkCircleOutline, CloseCircleOutline } from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import { useMessage } from 'naive-ui'
import { testApi, type ConnectionTestResult } from '@/api'

const store = useAppStore()
const message = useMessage()

// 监听 store.error 变化，显示错误消息
watch(() => store.error, (newError) => {
  if (newError) {
    message.error(newError)
  }
})

// 连接测试状态
const connectionTesting = ref(false)
const connectionResult = ref<ConnectionTestResult | null>(null)
const selectedModelForTest = ref<string>('')

// 测试启动时选择的模型
const selectedModelForRun = ref<string>('')

// 模型选项
const modelOptions = computed(() => {
  return store.config?.models.map(m => ({
    label: m.name + (m.skip ? ' (已禁用)' : ''),
    value: m.name
  })) || []
})

// 可用于测试的模型选项（优先显示启用的模型）
const modelOptionsForRun = computed(() => {
  const models = store.config?.models || []
  return models.map(m => ({
    label: m.name + (m.skip ? ' (已禁用)' : ''),
    value: m.name,
    disabled: false // 不禁用，让用户可以选择任何模型
  }))
})

// 当模型列表变化时，自动选择第一个启用的模型
watch(() => store.config?.models, (models) => {
  if (models && models.length > 0 && !selectedModelForRun.value) {
    const enabledModel = models.find(m => !m.skip)
    if (enabledModel) {
      selectedModelForRun.value = enabledModel.name
    } else if (models[0]) {
      selectedModelForRun.value = models[0].name
    }
  }
}, { immediate: true })

// 计算进度
const progressPercent = computed(() => {
  if (!store.testProgress || !store.testProgress.total_requests) return 0
  return Math.round((store.testProgress.current_requests || 0) / store.testProgress.total_requests * 100)
})

// 测试连接
async function handleTestConnection() {
  if (!selectedModelForTest.value) {
    message.warning('请先选择要测试的模型')
    return
  }
  
  connectionTesting.value = true
  connectionResult.value = null
  
  try {
    const res = await testApi.testConnection(selectedModelForTest.value)
    if (res.success && res.data) {
      connectionResult.value = res.data
      if (res.data.success) {
        message.success(`模型 ${selectedModelForTest.value} 连接成功！`)
      } else {
        message.error(`模型 ${selectedModelForTest.value} 连接失败：${res.data.message}`)
      }
    } else {
      message.error(res.error || '测试连接失败')
    }
  } catch (e: any) {
    message.error(e.message || '测试连接失败')
  } finally {
    connectionTesting.value = false
  }
}

// 开始测试
async function handleStartTest() {
  if (!selectedModelForRun.value) {
    message.warning('请先选择要测试的模型')
    return
  }
  
  try {
    const runId = await store.startTest(selectedModelForRun.value)
    message.success(`测试已启动，模型: ${selectedModelForRun.value}`)
  } catch (e: any) {
    message.error(e.message || '启动失败')
  }
}

// 停止测试
async function handleStopTest() {
  try {
    await store.stopTest()
    message.info('正在停止测试...')
  } catch (e: any) {
    message.error(e.message || '停止失败')
  }
}

// 格式化数字
function formatNumber(num: number | undefined, decimals: number = 2): string {
  if (num === undefined || num === null) return '-'
  return num.toFixed(decimals)
}
</script>

<template>
  <div class="test-control">
    <!-- 控制按钮 -->
    <NCard class="control-card">
      <div class="control-header">
        <div class="status-info">
          <span class="status-dot" :class="{ running: store.testRunning, idle: !store.testRunning }"></span>
          <span class="status-text">
            {{ store.testRunning ? '测试运行中' : '空闲' }}
          </span>
          <NTag size="small" :type="store.wsConnected ? 'success' : 'error'" v-if="store.testRunning">
            {{ store.wsConnected ? '实时连接' : '连接断开' }}
          </NTag>
          <NTag size="small" type="info" v-if="store.currentTestModelName">
            {{ store.currentTestModelName }}
          </NTag>
        </div>
        
        <NSpace align="center">
          <!-- 模型选择（仅在未运行时显示） -->
          <NSelect 
            v-if="!store.testRunning"
            v-model:value="selectedModelForRun"
            :options="modelOptionsForRun"
            placeholder="选择要测试的模型"
            style="width: 240px"
            filterable
          />
          
          <NButton 
            type="primary" 
            size="large"
            :disabled="store.testRunning || !selectedModelForRun"
            @click="handleStartTest"
          >
            <template #icon><NIcon><PlayOutline /></NIcon></template>
            开始测试
          </NButton>
          
          <NButton 
            type="error" 
            size="large"
            :disabled="!store.testRunning"
            @click="handleStopTest"
          >
            <template #icon><NIcon><StopOutline /></NIcon></template>
            停止测试
          </NButton>
        </NSpace>
      </div>
      
      <!-- 测试配置摘要 -->
      <div class="config-summary" v-if="store.config">
        <NTag>并发: {{ store.config.test.concurrency_levels?.join(', ') || store.config.test.concurrency }}</NTag>
        <NTag>时长: {{ store.config.test.duration }}</NTag>
        <NTag>流式: {{ store.config.prompt.stream ? '是' : '否' }}</NTag>
      </div>
    </NCard>
    
    <!-- 实时进度 - 测试运行时始终显示 -->
    <NCard class="progress-card" v-if="store.testRunning">
      <template #header>
        <div class="card-title">测试进度</div>
      </template>
      
      <div class="progress-info">
        <!-- 等待进度数据 -->
        <template v-if="!store.testProgress || store.testProgress.type === 'start'">
          <div class="waiting-progress">
            <NSpin size="small" />
            <span>正在初始化测试...</span>
          </div>
        </template>
        
        <!-- 有进度数据时显示详情 -->
        <template v-else>
          <div class="current-test">
            <span class="label">当前模型:</span>
            <span class="value">{{ store.testProgress.model_name || '-' }}</span>
            <NTag size="small" v-if="store.testProgress.concurrency">
              并发 {{ store.testProgress.concurrency }}
            </NTag>
            <NTag size="small" type="info" v-if="store.testProgress.context_tokens">
              上下文 {{ store.testProgress.context_tokens }} tokens
            </NTag>
          </div>
          
          <NProgress
            type="line"
            :percentage="progressPercent"
            :height="12"
            :border-radius="6"
            :fill-border-radius="6"
            indicator-placement="inside"
            processing
          />
          
          <div class="progress-stats">
            <span>{{ store.testProgress.current_requests || 0 }} / {{ store.testProgress.total_requests || 0 }} 请求</span>
            <span class="success">成功: {{ store.testProgress.success_requests || 0 }}</span>
            <span class="failed" v-if="store.testProgress.failed_requests">失败: {{ store.testProgress.failed_requests }}</span>
          </div>
        </template>
      </div>
      
      <!-- 实时指标 -->
      <NGrid :cols="6" :x-gap="12" :y-gap="16" class="realtime-metrics" v-if="store.testProgress && store.testProgress.type === 'progress'">
        <NGi>
          <div class="metric-card">
            <div class="metric-value">{{ formatNumber(store.testProgress.rps) }}</div>
            <div class="metric-label">RPS</div>
          </div>
        </NGi>
        <NGi>
          <div class="metric-card">
            <div class="metric-value">{{ formatNumber(store.testProgress.tps) }}</div>
            <div class="metric-label">总TPS</div>
          </div>
        </NGi>
        <NGi>
          <div class="metric-card">
            <div class="metric-value">{{ formatNumber(store.testProgress.prefill_tps) }}</div>
            <div class="metric-label">预填充速度</div>
          </div>
        </NGi>
        <NGi>
          <div class="metric-card">
            <div class="metric-value">{{ formatNumber(store.testProgress.decode_tps_avg) }}</div>
            <div class="metric-label">生成速度</div>
          </div>
        </NGi>
        <NGi>
          <div class="metric-card">
            <div class="metric-value">{{ formatNumber(store.testProgress.avg_ttft_ms) }}</div>
            <div class="metric-label">预填充耗时(TTFT)</div>
          </div>
        </NGi>
        <NGi>
          <div class="metric-card">
            <div class="metric-value">
              {{ store.testProgress.total_requests ? Math.round((store.testProgress.success_requests || 0) / store.testProgress.total_requests * 100) : 0 }}%
            </div>
            <div class="metric-label">成功率</div>
          </div>
        </NGi>
      </NGrid>
    </NCard>
    
    <!-- 连接测试 -->
    <NCard class="connection-card" v-if="!store.testRunning">
      <template #header>
        <div class="card-title">
          <NIcon><FlashOutline /></NIcon>
          测试连接
        </div>
      </template>
      
      <div class="connection-test">
        <div class="test-controls">
          <NSelect 
            v-model:value="selectedModelForTest"
            :options="modelOptions"
            placeholder="选择要测试的模型"
            style="width: 300px"
          />
          <NButton 
            type="info"
            :loading="connectionTesting"
            :disabled="!selectedModelForTest"
            @click="handleTestConnection"
          >
            <template #icon><NIcon><FlashOutline /></NIcon></template>
            测试连接
          </NButton>
        </div>
        
        <!-- 连接测试结果 -->
        <div class="connection-result" v-if="connectionResult">
          <NAlert :type="connectionResult.success ? 'success' : 'error'">
            <template #icon>
              <NIcon>
                <CheckmarkCircleOutline v-if="connectionResult.success" />
                <CloseCircleOutline v-else />
              </NIcon>
            </template>
            <template #header>
              {{ connectionResult.success ? '连接成功' : '连接失败' }}
            </template>
            
            <NDescriptions :column="2" size="small" label-placement="left">
              <NDescriptionsItem label="模型">{{ connectionResult.model_name }}</NDescriptionsItem>
              <NDescriptionsItem label="延迟">{{ connectionResult.latency_ms.toFixed(0) }} ms</NDescriptionsItem>
              <template v-if="connectionResult.success">
                <NDescriptionsItem label="Token数">{{ connectionResult.token_count || '-' }}</NDescriptionsItem>
                <NDescriptionsItem label="响应" :span="2">
                  <div class="response-preview">{{ connectionResult.response }}</div>
                </NDescriptionsItem>
              </template>
              <template v-else>
                <NDescriptionsItem label="错误信息" :span="2">{{ connectionResult.message }}</NDescriptionsItem>
              </template>
            </NDescriptions>
          </NAlert>
        </div>
      </div>
    </NCard>
    
    <!-- 空闲状态提示 -->
    <NCard v-if="!store.testRunning" class="idle-card">
      <div class="idle-content">
        <NIcon size="48" color="#3f3f46">
          <PlayOutline />
        </NIcon>
        <h3>准备就绪</h3>
        <p>点击"开始测试"按钮开始性能测试</p>
        
        <NAlert type="info" style="margin-top: 16px; text-align: left">
          <template #header>测试前请确认</template>
          <ul style="margin: 8px 0 0 16px; padding: 0;">
            <li>已配置至少一个启用的模型</li>
            <li>模型API地址可访问（可使用上方"测试连接"验证）</li>
            <li>API密钥正确</li>
          </ul>
        </NAlert>
      </div>
    </NCard>
  </div>
</template>

<style scoped>
.test-control {
  max-width: 900px;
}

.control-card {
  margin-bottom: 20px;
}

.control-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.status-info {
  display: flex;
  align-items: center;
  gap: 10px;
}

.status-text {
  font-size: 16px;
  font-weight: 500;
}

.config-summary {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.progress-card {
  margin-bottom: 20px;
}

.card-title {
  font-weight: 500;
}

.progress-info {
  margin-bottom: 20px;
}

.current-test {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.current-test .label {
  color: var(--text-muted);
}

.current-test .value {
  font-weight: 500;
}

.progress-stats {
  display: flex;
  gap: 16px;
  margin-top: 8px;
  font-size: 14px;
  color: var(--text-secondary);
}

.progress-stats .success {
  color: var(--success-color);
}

.progress-stats .failed {
  color: var(--error-color);
}

.realtime-metrics {
  margin-top: 20px;
}

.metric-card {
  padding: 16px;
  background: var(--bg-elevated);
  border-radius: 8px;
  text-align: center;
}

.metric-value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 24px;
  font-weight: 600;
  background: var(--gradient-primary);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.metric-label {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.idle-card {
  text-align: center;
  padding: 40px 20px;
}

.idle-content h3 {
  margin: 16px 0 8px;
  font-size: 18px;
  font-weight: 500;
}

.idle-content p {
  color: var(--text-muted);
  margin: 0;
}

.waiting-progress {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px;
  justify-content: center;
  color: var(--text-secondary);
}

.connection-card {
  margin-bottom: 20px;
}

.connection-card .card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.connection-test {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.test-controls {
  display: flex;
  gap: 12px;
  align-items: center;
}

.connection-result {
  margin-top: 8px;
}

.response-preview {
  max-height: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
  line-height: 1.5;
  color: var(--text-secondary);
}
</style>

