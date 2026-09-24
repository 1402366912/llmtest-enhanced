<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NCard, NSelect, NSpace, useMessage } from 'naive-ui'
import { useAppStore } from '@/stores/app'

const store = useAppStore()
const message = useMessage()
const frame = ref<HTMLIFrameElement | null>(null)
const selectedModelName = ref('')
const modelOptions = computed(() => (store.config?.models || []).map(model => ({
  label: model.name,
  value: model.name,
})))
const selectedModel = computed(() => (store.config?.models || []).find(
  model => model.name === selectedModelName.value,
))

watch(modelOptions, options => {
  if (!options.some(option => option.value === selectedModelName.value)) {
    selectedModelName.value = (store.config?.models || []).find(model => !model.skip)?.name
      || options[0]?.value || ''
  }
}, { immediate: true })

function chatEndpoint(baseUrl: string): string {
  const parsed = new URL(baseUrl)
  if (['localhost', '127.0.0.1', '::1'].includes(parsed.hostname)) {
    parsed.hostname = window.location.hostname
  }
  const url = parsed.toString().replace(/\/$/, '')
  if (url.endsWith('/chat/completions')) return url
  if (url.endsWith('/v1')) return `${url}/chat/completions`
  return `${url}/v1/chat/completions`
}

function syncModel(): void {
  const model = selectedModel.value
  if (!model || !frame.value?.contentWindow) {
    message.warning('请先选择模型并等待单请求工具加载')
    return
  }
  frame.value.contentWindow.postMessage({
    type: 'llmtest:model-config',
    apiUrl: chatEndpoint(model.base_url),
    modelName: String(model.params?.model || model.name),
    apiKey: model.api_key || '',
  }, window.location.origin)
  message.success('已把当前模型配置填入单请求工具')
}
</script>

<template>
  <NCard title="单请求基准 · v2.2" class="isolated-benchmark">
    <NSpace align="center" style="margin-bottom: 12px">
      <NSelect v-model:value="selectedModelName" :options="modelOptions" style="width: 300px" placeholder="选择模型" />
      <NButton type="primary" @click="syncModel">使用当前模型配置</NButton>
    </NSpace>
    <NAlert type="info" style="margin-bottom: 12px">
      本页保留 v2.2 的长度扫描、并发统计、图表、历史和导出。主测试页用于持续压测；这里按单请求首 token 计时。
    </NAlert>
    <iframe ref="frame" title="本地大模型推理速度测试工具 v2.2" src="/isolated-benchmark.html"
      style="width: 100%; min-height: 82vh; border: 1px solid #3f3f46; border-radius: 8px; background: white"
      @load="syncModel" />
  </NCard>
</template>
