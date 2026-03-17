<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { 
  NCard, NForm, NFormItem, NInput, NInputNumber, NSelect, NSwitch, NButton,
  NTabs, NTabPane, NSpace, NTag, NDynamicTags, NIcon, NCollapse, NCollapseItem,
  NDataTable, NPopconfirm, NModal, NAlert, useMessage
} from 'naive-ui'
import { AddOutline, TrashOutline, SaveOutline, RefreshOutline } from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import { datasetApi, type DatasetPreview } from '@/api'
import type { Config, ModelConfig, TestConfig, PromptConfig } from '@/types'

const store = useAppStore()
const message = useMessage()

// 本地编辑状态
const localConfig = ref<Config | null>(null)
const hasChanges = ref(false)

// 数据集预览状态
const datasetLoading = ref(false)
const datasetPreview = ref<DatasetPreview | null>(null)

// 模型编辑
const showModelModal = ref(false)
const editingModel = ref<ModelConfig | null>(null)
const modelForm = ref<ModelConfig>({
  name: '',
  type: 'openai',
  api_key: '',
  base_url: '',
  params: {
    model: '',
    temperature: 0.7,
    max_tokens: 2048,
    top_p: 1.0
  },
  skip: false
})

// 模型类型选项
const modelTypeOptions = [
  { label: 'OpenAI', value: 'openai' },
  { label: 'Anthropic', value: 'anthropic' },
  { label: 'Gemini', value: 'gemini' },
]

// 监听store配置变化
watch(() => store.config, (newConfig) => {
  if (newConfig) {
    localConfig.value = JSON.parse(JSON.stringify(newConfig))
    hasChanges.value = false
  }
}, { immediate: true, deep: true })

// 监听本地配置变化
watch(localConfig, () => {
  hasChanges.value = true
}, { deep: true })

// 模型表格列
const modelColumns = [
  { title: '名称', key: 'name', width: 150 },
  { title: '类型', key: 'type', width: 100 },
  { title: 'API地址', key: 'base_url', ellipsis: { tooltip: true } },
  { 
    title: '状态', 
    key: 'skip', 
    width: 80,
    render: (row: ModelConfig) => h(NTag, { 
      type: row.skip ? 'default' : 'success',
      size: 'small'
    }, { default: () => row.skip ? '禁用' : '启用' })
  },
  {
    title: '操作',
    key: 'actions',
    width: 120,
    render: (row: ModelConfig) => h(NSpace, null, {
      default: () => [
        h(NButton, { 
          size: 'small', 
          quaternary: true,
          onClick: () => openEditModelModal(row)
        }, { default: () => '编辑' }),
        h(NPopconfirm, {
          onPositiveClick: () => deleteModel(row.name)
        }, {
          trigger: () => h(NButton, { 
            size: 'small', 
            quaternary: true,
            type: 'error'
          }, { default: () => '删除' }),
          default: () => '确定删除该模型?'
        })
      ]
    })
  }
]

// 保存配置
async function saveConfig() {
  if (!localConfig.value) return
  
  try {
    await store.saveConfig(localConfig.value)
    hasChanges.value = false
    message.success('配置已保存')
  } catch (e: any) {
    message.error(e.message || '保存失败')
  }
}

// 重置配置
function resetConfig() {
  if (store.config) {
    localConfig.value = JSON.parse(JSON.stringify(store.config))
    hasChanges.value = false
  }
}

// 打开添加模型对话框
function openAddModelModal() {
  editingModel.value = null
  modelForm.value = {
    name: '',
    type: 'openai',
    api_key: '',
    base_url: 'http://localhost:8080/v1',
    params: {
      model: '',
      temperature: 0.7,
      max_tokens: 2048,
      top_p: 1.0
    },
    skip: false
  }
  showModelModal.value = true
}

// 打开编辑模型对话框
function openEditModelModal(model: ModelConfig) {
  editingModel.value = model
  modelForm.value = JSON.parse(JSON.stringify(model))
  showModelModal.value = true
}

// 保存模型
function saveModel() {
  if (!modelForm.value.name.trim()) {
    message.warning('请输入模型名称')
    return
  }
  if (!localConfig.value) return
  
  if (editingModel.value) {
    // 更新现有模型
    const index = localConfig.value.models.findIndex(m => m.name === editingModel.value!.name)
    if (index !== -1) {
      localConfig.value.models[index] = { ...modelForm.value }
    }
  } else {
    // 添加新模型
    if (localConfig.value.models.some(m => m.name === modelForm.value.name)) {
      message.warning('模型名称已存在')
      return
    }
    localConfig.value.models.push({ ...modelForm.value })
  }
  
  showModelModal.value = false
}

// 删除模型
function deleteModel(name: string) {
  if (!localConfig.value) return
  localConfig.value.models = localConfig.value.models.filter(m => m.name !== name)
}

// 切换模型启用状态
function toggleModel(model: ModelConfig) {
  model.skip = !model.skip
}

// 解析并发度列表字符串
function parseConcurrencyLevels(value: string): number[] {
  return value.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n) && n > 0)
}

// 格式化并发度列表
function formatConcurrencyLevels(levels: number[]): string {
  return levels.join(', ')
}

// 预览数据集
async function previewDataset() {
  datasetLoading.value = true
  datasetPreview.value = null
  
  try {
    // 先保存配置，确保后端使用最新的数据集路径
    if (hasChanges.value && localConfig.value) {
      await store.saveConfig(localConfig.value)
      hasChanges.value = false
    }
    
    const res = await datasetApi.preview()
    if (res.success && res.data) {
      datasetPreview.value = res.data
      if (res.data.exists) {
        message.success(`数据集加载成功，共 ${res.data.total_count} 条提示词`)
      } else {
        message.warning(res.data.error || '数据集加载失败')
      }
    } else {
      message.error(res.error || '预览失败')
    }
  } catch (e: any) {
    message.error(e.message || '预览失败')
  } finally {
    datasetLoading.value = false
  }
}

import { h } from 'vue'
</script>

<template>
  <div class="config-panel">
    <!-- 保存按钮栏 -->
    <div class="config-actions">
      <NSpace>
        <NButton 
          type="primary" 
          :disabled="!hasChanges"
          @click="saveConfig"
        >
          <template #icon><NIcon><SaveOutline /></NIcon></template>
          保存配置
        </NButton>
        <NButton 
          :disabled="!hasChanges"
          @click="resetConfig"
        >
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          重置
        </NButton>
      </NSpace>
      <NTag v-if="hasChanges" type="warning" size="small">有未保存的更改</NTag>
    </div>
    
    <NTabs type="line" animated v-if="localConfig">
      <!-- 测试参数 -->
      <NTabPane name="test" tab="测试参数">
        <NCard>
          <NForm label-placement="left" label-width="140">
            <NFormItem label="基础并发数">
              <NInputNumber 
                v-model:value="localConfig.test.concurrency" 
                :min="1" 
                :max="1000"
                style="width: 200px"
              />
            </NFormItem>
            
            <NFormItem label="并发度列表">
              <NDynamicTags 
                :value="(localConfig.test.concurrency_levels || []).map(String)"
                @update:value="(v: string[]) => localConfig!.test.concurrency_levels = v.map((s: string) => parseInt(s)).filter((n: number) => !isNaN(n) && n > 0)"
              />
              <template #feedback>
                点击添加并发度数值，测试将依次执行每个并发级别
              </template>
            </NFormItem>
            
            <NFormItem label="测试模式">
              <NSwitch 
                v-model:value="localConfig.test.stress_test_mode"
              >
                <template #checked>压力测试</template>
                <template #unchecked>请求数模式</template>
              </NSwitch>
              <template #feedback>
                <span v-if="localConfig.test.stress_test_mode">
                  压力测试模式：在指定时间内持续发送请求
                </span>
                <span v-else>
                  请求数模式：并发=1时发送10个请求，其他为 <strong>并发数×3</strong> 个请求
                </span>
              </template>
            </NFormItem>
            
            <NFormItem label="测试持续时间" v-if="localConfig.test.stress_test_mode">
              <NInput 
                v-model:value="localConfig.test.duration" 
                placeholder="如: 30s, 1m"
                style="width: 200px"
              />
              <template #feedback>
                压力测试模式下的测试持续时间
              </template>
            </NFormItem>
            
            <NFormItem label="预热时间">
              <NInput 
                v-model:value="localConfig.test.warmup_duration" 
                placeholder="如: 5s"
                style="width: 200px"
              />
            </NFormItem>
            
            <NFormItem label="请求超时">
              <NInput 
                v-model:value="localConfig.test.request_timeout" 
                placeholder="如: 120s"
                style="width: 200px"
              />
            </NFormItem>
            
            <NFormItem label="最大重试次数">
              <NInputNumber 
                v-model:value="localConfig.test.max_retries" 
                :min="0" 
                :max="10"
                style="width: 200px"
              />
            </NFormItem>
            
            <NFormItem label="显示进度条">
              <NSwitch v-model:value="localConfig.test.show_progress" />
            </NFormItem>
          </NForm>
          
          <NCollapse>
            <NCollapseItem title="上下文长度测试（高级）" name="context">
              <NForm label-placement="left" label-width="140">
                <NFormItem label="上下文起始Token">
                  <NInputNumber 
                    v-model:value="localConfig.test.context_token_start" 
                    :min="0"
                    style="width: 200px"
                    clearable
                  />
                </NFormItem>
                
                <NFormItem label="上下文结束Token">
                  <NInputNumber 
                    v-model:value="localConfig.test.context_token_end" 
                    :min="0"
                    style="width: 200px"
                    clearable
                  />
                </NFormItem>
                
                <NFormItem label="容差范围">
                  <NInputNumber 
                    v-model:value="localConfig.test.context_tolerance" 
                    :min="0"
                    :max="1"
                    :step="0.05"
                    style="width: 200px"
                  />
                  <template #feedback>
                    上下文长度的容差比例，如0.1表示±10%
                  </template>
                </NFormItem>
                
                <NFormItem label="Prefill指标">
                  <NSwitch v-model:value="localConfig.test.enable_prefill_metrics" />
                  <template #feedback>
                    启用Prefill/Decode阶段指标统计（需流式输出）
                  </template>
                </NFormItem>
              </NForm>
            </NCollapseItem>
          </NCollapse>
        </NCard>
      </NTabPane>
      
      <!-- 模型配置 -->
      <NTabPane name="models" tab="模型配置">
        <NCard>
          <template #header>
            <div class="card-header">
              <span>已配置模型</span>
              <NButton type="primary" size="small" @click="openAddModelModal">
                <template #icon><NIcon><AddOutline /></NIcon></template>
                添加模型
              </NButton>
            </div>
          </template>
          
          <NDataTable
            :columns="modelColumns"
            :data="localConfig.models"
            :row-key="(row: ModelConfig) => row.name"
            :bordered="false"
            size="small"
          />
        </NCard>
      </NTabPane>
      
      <!-- 提示词配置 -->
      <NTabPane name="prompt" tab="提示词">
        <NCard>
          <NForm label-placement="top">
            <NFormItem label="系统消息">
              <NInput 
                v-model:value="localConfig.prompt.system_message" 
                type="textarea"
                :rows="3"
                placeholder="设置AI的角色和行为..."
              />
            </NFormItem>
            
            <NFormItem label="用户消息">
              <NInput 
                v-model:value="localConfig.prompt.user_message" 
                type="textarea"
                :rows="4"
                placeholder="测试时发送的用户消息..."
              />
            </NFormItem>
            
            <NFormItem label="数据集路径">
              <div class="dataset-input">
                <NInput 
                  v-model:value="localConfig.prompt.dataset_path" 
                  placeholder="如: dataset/prompts.txt（每行一个提示词）"
                  style="flex: 1"
                />
                <NButton 
                  type="info" 
                  ghost 
                  :loading="datasetLoading"
                  @click="previewDataset"
                >
                  预览
                </NButton>
              </div>
              <template #feedback>
                如果指定了数据集，将随机选取其中的提示词进行测试
              </template>
            </NFormItem>
            
            <!-- 数据集预览 -->
            <div class="dataset-preview" v-if="datasetPreview">
              <NAlert 
                :type="datasetPreview.exists ? 'success' : 'error'"
                :title="datasetPreview.exists ? `数据集加载成功 (${datasetPreview.total_count} 条)` : '数据集加载失败'"
              >
                <template v-if="datasetPreview.error">
                  {{ datasetPreview.error }}
                </template>
                <template v-else-if="datasetPreview.samples && datasetPreview.samples.length > 0">
                  <div class="preview-header">前 {{ datasetPreview.samples.length }} 条样本预览：</div>
                  <div class="preview-samples">
                    <div 
                      v-for="(sample, index) in datasetPreview.samples" 
                      :key="index"
                      class="preview-sample"
                    >
                      <span class="sample-index">{{ index + 1 }}.</span>
                      <span class="sample-text">{{ sample }}</span>
                    </div>
                  </div>
                </template>
              </NAlert>
            </div>
            
            <NFormItem label="流式输出">
              <NSwitch v-model:value="localConfig.prompt.stream" />
            </NFormItem>
            
            <NCollapse>
              <NCollapseItem title="多模态配置（可选）" name="multimodal">
                <NForm label-placement="left" label-width="100">
                  <NFormItem label="图片路径">
                    <NInput 
                      v-model:value="localConfig.prompt.image_path" 
                      placeholder="本地图片文件路径"
                      clearable
                    />
                  </NFormItem>
                  
                  <NFormItem label="图片URL">
                    <NInput 
                      v-model:value="localConfig.prompt.image_url" 
                      placeholder="图片网络地址"
                      clearable
                    />
                  </NFormItem>
                </NForm>
              </NCollapseItem>
            </NCollapse>
          </NForm>
        </NCard>
      </NTabPane>
    </NTabs>
    
    <!-- 模型编辑对话框 -->
    <NModal
      v-model:show="showModelModal"
      preset="card"
      :title="editingModel ? '编辑模型' : '添加模型'"
      style="width: 600px"
    >
      <NForm label-placement="left" label-width="100">
        <NFormItem label="模型名称" required>
          <NInput 
            v-model:value="modelForm.name" 
            placeholder="自定义名称，如: gpt-4-turbo"
            :disabled="!!editingModel"
          />
        </NFormItem>
        
        <NFormItem label="模型类型" required>
          <NSelect 
            v-model:value="modelForm.type" 
            :options="modelTypeOptions"
            style="width: 200px"
          />
        </NFormItem>
        
        <NFormItem label="API密钥" required>
          <NInput 
            v-model:value="modelForm.api_key" 
            type="password"
            show-password-on="click"
            placeholder="API Key"
          />
        </NFormItem>
        
        <NFormItem label="API地址">
          <NInput 
            v-model:value="modelForm.base_url" 
            placeholder="如: https://api.openai.com/v1"
          />
        </NFormItem>
        
        <NFormItem label="模型标识">
          <NInput 
            v-model:value="modelForm.params.model" 
            placeholder="如: gpt-4-turbo"
          />
        </NFormItem>
        
        <NFormItem label="Temperature">
          <NInputNumber 
            v-model:value="modelForm.params.temperature" 
            :min="0" 
            :max="2"
            :step="0.1"
            style="width: 150px"
          />
        </NFormItem>
        
        <NFormItem label="Max Tokens">
          <NInputNumber 
            v-model:value="modelForm.params.max_tokens" 
            :min="1" 
            :max="128000"
            style="width: 150px"
          />
        </NFormItem>
        
        <NFormItem label="Top P">
          <NInputNumber 
            v-model:value="modelForm.params.top_p" 
            :min="0" 
            :max="1"
            :step="0.1"
            style="width: 150px"
          />
        </NFormItem>
        
        <NFormItem label="启用">
          <NSwitch v-model:value="modelForm.skip" :checked-value="false" :unchecked-value="true" />
        </NFormItem>
      </NForm>
      
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModelModal = false">取消</NButton>
          <NButton type="primary" @click="saveModel">保存</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.config-panel {
  max-width: 900px;
}

.config-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  padding: 12px 16px;
  background: var(--bg-card);
  border-radius: 8px;
  border: 1px solid var(--border-color);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

:deep(.n-card) {
  margin-bottom: 16px;
}

:deep(.n-collapse) {
  margin-top: 16px;
}

/* 数据集输入框 */
.dataset-input {
  display: flex;
  gap: 8px;
  width: 100%;
}

/* 数据集预览 */
.dataset-preview {
  margin: 12px 0 16px;
}

.preview-header {
  font-size: 13px;
  color: var(--text-muted);
  margin-bottom: 8px;
}

.preview-samples {
  max-height: 300px;
  overflow-y: auto;
  background: var(--bg-darker);
  border-radius: 6px;
  padding: 8px;
}

.preview-sample {
  display: flex;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  margin-bottom: 4px;
  font-size: 13px;
  line-height: 1.5;
}

.preview-sample:hover {
  background: rgba(255, 255, 255, 0.05);
}

.preview-sample:last-child {
  margin-bottom: 0;
}

.sample-index {
  flex-shrink: 0;
  color: #6366f1;
  font-weight: 500;
  min-width: 24px;
}

.sample-text {
  color: var(--text-secondary);
  word-break: break-all;
}
</style>

