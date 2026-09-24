<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { 
  NLayout, NLayoutSider, NLayoutContent, NMenu, NButton, NIcon, NSpin, NTabs, NTabPane,
  NModal, NForm, NFormItem, NInput, NSpace, NCollapse, NCollapseItem, useMessage, useDialog,
  NResult, NDescriptions, NDescriptionsItem
} from 'naive-ui'
import { 
  ServerOutline, AddOutline, SettingsOutline, HardwareChipOutline,
  PlayOutline, StopOutline, BarChartOutline, TimeOutline, TrophyOutline,
  CubeOutline, ChevronDownOutline, ChevronForwardOutline, AnalyticsOutline,
  CloudDownloadOutline, CloudUploadOutline
} from '@vicons/ionicons5'
import { useAppStore } from '@/stores/app'
import { dataTransferApi, type ImportResult } from '@/api'
import ConfigPanel from '@/components/ConfigPanel.vue'
import TestControl from '@/components/TestControl.vue'
import ResultsPanel from '@/components/ResultsPanel.vue'
import HistoryPanel from '@/components/HistoryPanel.vue'
import LeaderboardPanel from '@/components/LeaderboardPanel.vue'
import AnalyticsPanel from '@/components/AnalyticsPanel.vue'
import IsolatedBenchmark from '@/components/IsolatedBenchmark.vue'

const store = useAppStore()
const message = useMessage()
const dialog = useDialog()

const sidebarCollapsed = ref(false)
const showMachineModal = ref(false)
const editingMachine = ref<any>(null)
const machineForm = ref({
  name: '',
  description: '',
  gpu_model: ''
})

// 后端相关状态
const showBackendModal = ref(false)
const editingBackend = ref<any>(null)
const backendForm = ref({
  name: '',
  description: ''
})
const expandedMachines = ref<string[]>([])

const activeTab = ref('config')

// 机器列表菜单项
const machineMenuOptions = computed(() => {
  return store.machines.map(machine => ({
    label: machine.name,
    key: machine.id,
    icon: () => h(NIcon, null, { default: () => h(HardwareChipOutline) }),
    extra: machine.gpu_model || ''
  }))
})

// 当前选中的机器
const selectedMachineKey = computed({
  get: () => store.currentMachineId,
  set: (val) => {
    if (val && val !== store.currentMachineId) {
      handleSelectMachine(val as string)
    }
  }
})

// 选择机器
async function handleSelectMachine(id: string) {
  try {
    await store.selectMachine(id)
    message.success('已切换到机器: ' + store.currentMachine?.name)
  } catch (e: any) {
    message.error(e.message || '切换失败')
  }
}

// 打开添加机器对话框
function openAddMachineModal() {
  editingMachine.value = null
  machineForm.value = { name: '', description: '', gpu_model: '' }
  showMachineModal.value = true
}

// 打开编辑机器对话框
function openEditMachineModal(machine: any) {
  editingMachine.value = machine
  machineForm.value = {
    name: machine.name,
    description: machine.description || '',
    gpu_model: machine.gpu_model || ''
  }
  showMachineModal.value = true
}

// 保存机器
async function saveMachine() {
  if (!machineForm.value.name.trim()) {
    message.warning('请输入机器名称')
    return
  }
  
  try {
    if (editingMachine.value) {
      await store.updateMachine(editingMachine.value.id, machineForm.value)
      message.success('机器已更新')
    } else {
      await store.createMachine(machineForm.value)
      message.success('机器已创建')
    }
    showMachineModal.value = false
  } catch (e: any) {
    message.error(e.message || '操作失败')
  }
}

// 删除机器
function confirmDeleteMachine(machine: any) {
  dialog.warning({
    title: '确认删除',
    content: `确定要删除机器 "${machine.name}" 吗？机器数据将保留。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.deleteMachine(machine.id)
        message.success('机器已删除')
      } catch (e: any) {
        message.error(e.message || '删除失败')
      }
    }
  })
}

// 后端相关方法

// 切换机器展开状态
function toggleMachineExpand(machineId: string) {
  const index = expandedMachines.value.indexOf(machineId)
  if (index === -1) {
    expandedMachines.value.push(machineId)
    // 加载该机器的后端列表
    if (machineId === store.currentMachineId) {
      store.loadBackends(machineId)
    }
  } else {
    expandedMachines.value.splice(index, 1)
  }
}

// 检查机器是否展开
function isMachineExpanded(machineId: string) {
  return expandedMachines.value.includes(machineId)
}

// 选择后端
async function handleSelectBackend(machineId: string, backendId: string) {
  try {
    // 如果不是当前机器，先切换机器
    if (machineId !== store.currentMachineId) {
      await store.selectMachine(machineId)
    }
    await store.selectBackend(backendId)
    message.success('已切换到后端: ' + store.currentBackend?.name)
  } catch (e: any) {
    message.error(e.message || '切换失败')
  }
}

// 打开添加后端对话框
function openAddBackendModal() {
  if (!store.currentMachineId) {
    message.warning('请先选择机器')
    return
  }
  editingBackend.value = null
  backendForm.value = { name: '', description: '' }
  showBackendModal.value = true
}

// 打开编辑后端对话框
function openEditBackendModal(backend: any) {
  editingBackend.value = backend
  backendForm.value = {
    name: backend.name,
    description: backend.description || ''
  }
  showBackendModal.value = true
}

// 保存后端
async function saveBackend() {
  if (!backendForm.value.name.trim()) {
    message.warning('请输入后端名称')
    return
  }
  
  try {
    if (editingBackend.value) {
      await store.updateBackend(editingBackend.value.id, backendForm.value)
      message.success('后端已更新')
    } else {
      await store.createBackend(backendForm.value)
      message.success('后端已创建')
    }
    showBackendModal.value = false
  } catch (e: any) {
    message.error(e.message || '操作失败')
  }
}

// 删除后端
function confirmDeleteBackend(backend: any) {
  dialog.warning({
    title: '确认删除',
    content: `确定要删除后端 "${backend.name}" 吗？后端数据将保留。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.deleteBackend(backend.id)
        message.success('后端已删除')
      } catch (e: any) {
        message.error(e.message || '删除失败')
      }
    }
  })
}

// 监听当前机器变化，自动展开
watch(() => store.currentMachineId, (newId) => {
  if (newId && !expandedMachines.value.includes(newId)) {
    expandedMachines.value.push(newId)
  }
}, { immediate: true })

// 数据导入导出相关
const fileInputRef = ref<HTMLInputElement | null>(null)
const importing = ref(false)
const showImportResultModal = ref(false)
const importResult = ref<ImportResult | null>(null)
const showExportModal = ref(false)
const exportFileName = ref('')

// 天梯图弹窗
const showLeaderboardModal = ref(false)

// 打开导出对话框
function openExportModal() {
  // 生成默认文件名
  const now = new Date()
  const dateStr = now.toISOString().slice(0, 10).replace(/-/g, '')
  const timeStr = now.toTimeString().slice(0, 8).replace(/:/g, '')
  exportFileName.value = `llm-test-data-${dateStr}_${timeStr}`
  showExportModal.value = true
}

// 确认导出
function confirmExport() {
  const name = exportFileName.value.trim()
  if (!name) {
    message.warning('请输入文件名')
    return
  }
  showExportModal.value = false
  message.info('正在导出数据...')
  dataTransferApi.exportData(name)
}

// 触发文件选择
function triggerImport() {
  fileInputRef.value?.click()
}

// 处理文件上传
async function handleFileSelect(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  
  if (!file) return
  
  // 验证文件类型
  if (!file.name.toLowerCase().endsWith('.zip')) {
    message.error('请上传ZIP文件')
    input.value = ''
    return
  }
  
  importing.value = true
  
  try {
    const res = await dataTransferApi.importData(file)
    if (res.success && res.data) {
      importResult.value = res.data
      showImportResultModal.value = true
      // 刷新机器列表
      await store.loadMachines()
      message.success('数据导入成功')
    } else {
      message.error(res.error || '导入失败')
    }
  } catch (e: any) {
    message.error(e.message || '导入失败')
  } finally {
    importing.value = false
    input.value = ''
  }
}

// 渲染函数
import { h } from 'vue'
</script>

<template>
  <NLayout has-sider class="main-layout">
    <!-- 侧边栏 -->
    <NLayoutSider
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      :collapsed="sidebarCollapsed"
      show-trigger
      @collapse="sidebarCollapsed = true"
      @expand="sidebarCollapsed = false"
      class="sidebar"
    >
      <div class="sidebar-header">
        <div class="logo" v-if="!sidebarCollapsed">
          <div class="logo-icon">
            <NIcon size="24" color="#6366f1">
              <BarChartOutline />
            </NIcon>
          </div>
          <span class="logo-text">LLM Test</span>
        </div>
        <div class="logo-mini" v-else>
          <NIcon size="24" color="#6366f1">
            <BarChartOutline />
          </NIcon>
        </div>
      </div>
      
      <div class="sidebar-section" v-if="!sidebarCollapsed">
        <div class="section-title">
          <NIcon size="16"><ServerOutline /></NIcon>
          <span>测试机器</span>
        </div>
      </div>
      
      <!-- 机器和后端列表 -->
      <div class="machine-list" v-if="!sidebarCollapsed">
        <div 
          v-for="machine in store.machines" 
          :key="machine.id"
          class="machine-item"
          :class="{ active: machine.id === store.currentMachineId }"
        >
          <div 
            class="machine-header"
            @click="handleSelectMachine(machine.id)"
          >
            <div class="machine-info">
              <NIcon size="18" class="machine-icon"><HardwareChipOutline /></NIcon>
              <span class="machine-name">{{ machine.name }}</span>
            </div>
            <NButton 
              quaternary 
              circle 
              size="tiny"
              @click.stop="toggleMachineExpand(machine.id)"
              class="expand-btn"
            >
              <template #icon>
                <NIcon size="14">
                  <ChevronDownOutline v-if="isMachineExpanded(machine.id)" />
                  <ChevronForwardOutline v-else />
                </NIcon>
              </template>
            </NButton>
          </div>
          
          <!-- 后端列表 -->
          <div class="backend-list" v-if="isMachineExpanded(machine.id) && machine.id === store.currentMachineId">
            <div 
              v-for="backend in store.backends" 
              :key="backend.id"
              class="backend-item"
              :class="{ active: backend.id === store.currentBackendId }"
              @click="handleSelectBackend(machine.id, backend.id)"
            >
              <NIcon size="14" class="backend-icon"><CubeOutline /></NIcon>
              <span class="backend-name">{{ backend.name }}</span>
            </div>
            <div class="backend-add" @click="openAddBackendModal">
              <NIcon size="14"><AddOutline /></NIcon>
              <span>添加后端</span>
            </div>
          </div>
        </div>
      </div>
      
      <!-- 折叠时的菜单 -->
      <NMenu
        v-else
        :collapsed="sidebarCollapsed"
        :collapsed-width="64"
        :collapsed-icon-size="20"
        :options="machineMenuOptions"
        v-model:value="selectedMachineKey"
      />
      
      <!-- 天梯图按钮 -->
      <div class="sidebar-leaderboard" v-if="!sidebarCollapsed">
        <NButton 
          quaternary 
          block 
          size="small"
          @click="showLeaderboardModal = true"
        >
          <template #icon>
            <NIcon color="#f59e0b"><TrophyOutline /></NIcon>
          </template>
          全局天梯图
        </NButton>
      </div>
      <div class="sidebar-leaderboard-mini" v-else>
        <NButton 
          quaternary 
          circle 
          size="small"
          @click="showLeaderboardModal = true"
        >
          <template #icon>
            <NIcon color="#f59e0b"><TrophyOutline /></NIcon>
          </template>
        </NButton>
      </div>
      
      <div class="sidebar-actions" v-if="!sidebarCollapsed">
        <NButton quaternary block size="small" @click="openAddMachineModal">
          <template #icon>
            <NIcon><AddOutline /></NIcon>
          </template>
          添加机器
        </NButton>
      </div>
      <div class="sidebar-actions-mini" v-else>
        <NButton quaternary circle size="small" @click="openAddMachineModal">
          <template #icon>
            <NIcon><AddOutline /></NIcon>
          </template>
        </NButton>
      </div>
    </NLayoutSider>
    
    <!-- 主内容区 -->
    <NLayoutContent class="main-content">
      <NSpin :show="store.loading">
        <!-- 顶部标题栏 -->
        <div class="content-header">
          <div class="header-info">
            <h1 class="page-title">
              {{ store.currentMachine?.name || '选择测试机器' }}
              <span class="backend-badge" v-if="store.currentBackend">
                / {{ store.currentBackend.name }}
              </span>
            </h1>
            <span class="header-subtitle" v-if="store.currentMachine?.gpu_model">
              {{ store.currentMachine.gpu_model }}
            </span>
          </div>
          <div class="header-actions">
            <NButton 
              quaternary 
              size="small"
              @click="openExportModal"
            >
              <template #icon>
                <NIcon><CloudDownloadOutline /></NIcon>
              </template>
              导出数据
            </NButton>
            <NButton 
              quaternary 
              size="small"
              :loading="importing"
              @click="triggerImport"
            >
              <template #icon>
                <NIcon><CloudUploadOutline /></NIcon>
              </template>
              导入数据
            </NButton>
            <input 
              ref="fileInputRef"
              type="file" 
              accept=".zip"
              style="display: none"
              @change="handleFileSelect"
            />
            <NButton 
              v-if="store.currentBackend"
              quaternary 
              size="small"
              @click="openEditBackendModal(store.currentBackend)"
            >
              <template #icon>
                <NIcon><CubeOutline /></NIcon>
              </template>
              编辑后端
            </NButton>
            <NButton 
              v-if="store.currentMachine"
              quaternary 
              size="small"
              @click="openEditMachineModal(store.currentMachine)"
            >
              <template #icon>
                <NIcon><SettingsOutline /></NIcon>
              </template>
              编辑机器
            </NButton>
          </div>
        </div>
        
        <!-- 内容区域 -->
        <div class="content-body" v-if="store.currentMachine">
          <NTabs v-model:value="activeTab" type="line" animated>
            <NTabPane name="config" tab="配置">
              <template #tab>
                <div class="tab-label">
                  <NIcon size="16"><SettingsOutline /></NIcon>
                  <span>测试配置</span>
                </div>
              </template>
              <ConfigPanel />
            </NTabPane>
            
            <NTabPane name="test" tab="测试">
              <template #tab>
                <div class="tab-label">
                  <NIcon size="16"><PlayOutline /></NIcon>
                  <span>执行测试</span>
                </div>
              </template>
              <TestControl />
            </NTabPane>
            
            <NTabPane name="isolated" tab="单请求基准">
              <template #tab>
                <div class="tab-label">
                  <NIcon size="16"><BarChartOutline /></NIcon>
                  <span>单请求基准</span>
                </div>
              </template>
              <IsolatedBenchmark />
            </NTabPane>

            <NTabPane name="results" tab="结果">
              <template #tab>
                <div class="tab-label">
                  <NIcon size="16"><BarChartOutline /></NIcon>
                  <span>测试结果</span>
                </div>
              </template>
              <ResultsPanel />
            </NTabPane>
            
            <NTabPane name="history" tab="历史">
              <template #tab>
                <div class="tab-label">
                  <NIcon size="16"><TimeOutline /></NIcon>
                  <span>历史记录</span>
                </div>
              </template>
              <HistoryPanel />
            </NTabPane>
            
            <NTabPane name="analytics" tab="数据分析">
              <template #tab>
                <div class="tab-label">
                  <NIcon size="16"><AnalyticsOutline /></NIcon>
                  <span>数据分析</span>
                </div>
              </template>
              <AnalyticsPanel />
            </NTabPane>
          </NTabs>
        </div>
        
        <!-- 无机器提示 -->
        <div class="no-machine" v-else>
          <div class="empty-state">
            <NIcon size="64" color="#3f3f46">
              <ServerOutline />
            </NIcon>
            <h2>请选择或创建测试机器</h2>
            <p>在左侧边栏选择已有机器，或点击下方按钮创建新机器</p>
            <NButton type="primary" @click="openAddMachineModal">
              <template #icon>
                <NIcon><AddOutline /></NIcon>
              </template>
              创建机器
            </NButton>
          </div>
        </div>
      </NSpin>
    </NLayoutContent>
    
    <!-- 机器编辑对话框 -->
    <NModal
      v-model:show="showMachineModal"
      preset="card"
      :title="editingMachine ? '编辑机器' : '添加机器'"
      style="width: 480px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem label="名称" required>
          <NInput v-model:value="machineForm.name" placeholder="如：A100服务器" />
        </NFormItem>
        <NFormItem label="GPU型号">
          <NInput v-model:value="machineForm.gpu_model" placeholder="如：NVIDIA A100 80GB" />
        </NFormItem>
        <NFormItem label="描述">
          <NInput 
            v-model:value="machineForm.description" 
            type="textarea" 
            placeholder="机器描述（可选）"
            :rows="3"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton v-if="editingMachine" type="error" ghost @click="confirmDeleteMachine(editingMachine)">
            删除机器
          </NButton>
          <NButton @click="showMachineModal = false">取消</NButton>
          <NButton type="primary" @click="saveMachine">保存</NButton>
        </NSpace>
      </template>
    </NModal>
    
    <!-- 后端编辑对话框 -->
    <NModal
      v-model:show="showBackendModal"
      preset="card"
      :title="editingBackend ? '编辑后端' : '添加后端'"
      style="width: 480px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem label="名称" required>
          <NInput v-model:value="backendForm.name" placeholder="如：vLLM、SGLang、LMDeploy" />
        </NFormItem>
        <NFormItem label="描述">
          <NInput 
            v-model:value="backendForm.description" 
            type="textarea" 
            placeholder="后端描述（可选）"
            :rows="3"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton v-if="editingBackend" type="error" ghost @click="confirmDeleteBackend(editingBackend)">
            删除后端
          </NButton>
          <NButton @click="showBackendModal = false">取消</NButton>
          <NButton type="primary" @click="saveBackend">保存</NButton>
        </NSpace>
      </template>
    </NModal>
    
    <!-- 导出对话框 -->
    <NModal
      v-model:show="showExportModal"
      preset="card"
      title="导出数据"
      style="width: 450px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem label="文件名">
          <NInput 
            v-model:value="exportFileName" 
            placeholder="输入导出文件名"
          >
            <template #suffix>.zip</template>
          </NInput>
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showExportModal = false">取消</NButton>
          <NButton type="primary" @click="confirmExport">导出</NButton>
        </NSpace>
      </template>
    </NModal>
    
    <!-- 导入结果对话框 -->
    <NModal
      v-model:show="showImportResultModal"
      preset="card"
      title="导入完成"
      style="width: 500px"
    >
      <NResult
        status="success"
        title="数据导入成功"
        description="以下是导入结果统计"
      >
        <template #footer>
          <NDescriptions label-placement="left" :column="2" v-if="importResult">
            <NDescriptionsItem label="新增机器">
              {{ importResult.machines_added }}
            </NDescriptionsItem>
            <NDescriptionsItem label="合并机器">
              {{ importResult.machines_merged }}
            </NDescriptionsItem>
            <NDescriptionsItem label="新增后端">
              {{ importResult.backends_added }}
            </NDescriptionsItem>
            <NDescriptionsItem label="合并后端">
              {{ importResult.backends_merged }}
            </NDescriptionsItem>
            <NDescriptionsItem label="新增测试结果">
              {{ importResult.results_added }}
            </NDescriptionsItem>
            <NDescriptionsItem label="跳过重复结果">
              {{ importResult.results_skipped }}
            </NDescriptionsItem>
          </NDescriptions>
          <div v-if="importResult?.errors?.length" class="import-errors">
            <p style="color: #f59e0b; margin-top: 16px;">警告信息：</p>
            <ul>
              <li v-for="(err, idx) in importResult.errors" :key="idx">{{ err }}</li>
            </ul>
          </div>
          <NButton type="primary" @click="showImportResultModal = false" style="margin-top: 16px;">
            确定
          </NButton>
        </template>
      </NResult>
    </NModal>
    
    <!-- 天梯图弹窗 -->
    <NModal
      v-model:show="showLeaderboardModal"
      preset="card"
      title="全局性能天梯图"
      style="width: 90vw; max-width: 1200px"
      :style="{ maxHeight: '90vh' }"
    >
      <div class="leaderboard-modal-content">
        <LeaderboardPanel />
      </div>
    </NModal>
  </NLayout>
</template>

<style scoped>
.main-layout {
  height: 100vh;
  background: var(--bg-dark);
}

.sidebar {
  background: var(--bg-darker) !important;
  border-right: 1px solid var(--border-color) !important;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid var(--border-color);
}

.logo {
  display: flex;
  align-items: center;
  gap: 10px;
}

.logo-icon {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(99, 102, 241, 0.1);
  border-radius: 8px;
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
  background: var(--gradient-primary);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.logo-mini {
  display: flex;
  align-items: center;
  justify-content: center;
}

.sidebar-section {
  padding: 16px 16px 8px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.sidebar-actions {
  padding: 8px 12px;
  border-top: 1px solid var(--border-color);
  margin-top: auto;
}

.sidebar-actions-mini {
  padding: 8px;
  display: flex;
  justify-content: center;
  border-top: 1px solid var(--border-color);
  margin-top: auto;
}

.main-content {
  background: var(--bg-dark);
  display: flex;
  flex-direction: column;
}

.content-header {
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-darker);
}

.header-info {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  margin: 0;
}

.header-subtitle {
  font-size: 14px;
  color: var(--text-muted);
  padding: 4px 10px;
  background: var(--bg-elevated);
  border-radius: 4px;
}

.content-body {
  flex: 1;
  padding: 20px 24px;
  overflow-y: auto;
}

.tab-label {
  display: flex;
  align-items: center;
  gap: 6px;
}

.no-machine {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-state {
  text-align: center;
  padding: 40px;
}

.empty-state h2 {
  margin: 20px 0 8px;
  font-size: 20px;
  font-weight: 500;
}

.empty-state p {
  color: var(--text-muted);
  margin-bottom: 24px;
}

/* 机器和后端列表样式 */
.machine-list {
  padding: 8px 0;
  overflow-y: auto;
  flex: 1;
}

.machine-item {
  margin: 2px 8px;
  border-radius: 8px;
  overflow: hidden;
}

.machine-item.active {
  background: rgba(99, 102, 241, 0.1);
}

.machine-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  cursor: pointer;
  transition: background 0.2s;
  border-radius: 8px;
}

.machine-header:hover {
  background: rgba(255, 255, 255, 0.05);
}

.machine-item.active .machine-header {
  color: #6366f1;
}

.machine-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.machine-icon {
  flex-shrink: 0;
  color: #a1a1aa;
}

.machine-item.active .machine-icon {
  color: #6366f1;
}

.machine-name {
  font-size: 14px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.expand-btn {
  opacity: 0.5;
  transition: opacity 0.2s;
}

.machine-header:hover .expand-btn {
  opacity: 1;
}

/* 后端列表样式 */
.backend-list {
  padding: 4px 0 8px 32px;
}

.backend-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  border-radius: 6px;
  font-size: 13px;
  color: #a1a1aa;
  transition: all 0.2s;
}

.backend-item:hover {
  background: rgba(255, 255, 255, 0.05);
  color: #fafafa;
}

.backend-item.active {
  background: rgba(99, 102, 241, 0.15);
  color: #818cf8;
}

.backend-icon {
  flex-shrink: 0;
}

.backend-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.backend-add {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  cursor: pointer;
  border-radius: 6px;
  font-size: 12px;
  color: #71717a;
  transition: all 0.2s;
  margin-top: 4px;
}

.backend-add:hover {
  background: rgba(255, 255, 255, 0.05);
  color: #a1a1aa;
}

/* 标题中的后端标识 */
.backend-badge {
  font-size: 18px;
  font-weight: 500;
  color: #818cf8;
}

/* 天梯图按钮样式 */
.sidebar-leaderboard {
  padding: 8px 12px;
  border-top: 1px solid var(--border-color);
}

.sidebar-leaderboard-mini {
  padding: 8px;
  display: flex;
  justify-content: center;
  border-top: 1px solid var(--border-color);
}

/* 天梯图弹窗内容 */
.leaderboard-modal-content {
  max-height: 70vh;
  overflow-y: auto;
}
</style>
