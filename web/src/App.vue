<script setup lang="ts">
import { onMounted } from 'vue'
import { NConfigProvider, NMessageProvider, NDialogProvider, darkTheme, type GlobalThemeOverrides } from 'naive-ui'
import { useAppStore } from '@/stores/app'
import MainLayout from '@/views/MainLayout.vue'

const store = useAppStore()

// 自定义主题
const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#6366f1',
    primaryColorHover: '#818cf8',
    primaryColorPressed: '#4f46e5',
    primaryColorSuppl: '#6366f1',
    successColor: '#10b981',
    errorColor: '#ef4444',
    warningColor: '#f59e0b',
    infoColor: '#0ea5e9',
    borderRadius: '8px',
    borderRadiusSmall: '6px',
    fontFamily: 'Outfit, -apple-system, BlinkMacSystemFont, sans-serif',
    fontFamilyMono: 'JetBrains Mono, Fira Code, Consolas, monospace',
  },
  Card: {
    color: '#18181b',
    borderColor: '#27272a',
    borderRadius: '12px',
  },
  Button: {
    borderRadiusMedium: '8px',
    borderRadiusSmall: '6px',
  },
  Input: {
    borderRadius: '8px',
    color: '#27272a',
    borderColor: '#3f3f46',
    borderHover: '#6366f1',
    borderFocus: '#6366f1',
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '8px',
        color: '#27272a',
      }
    }
  },
  DataTable: {
    thColor: '#1f1f23',
    tdColor: '#18181b',
    borderColor: '#27272a',
  },
  Tabs: {
    tabTextColorActiveLine: '#6366f1',
    barColor: '#6366f1',
  },
  Menu: {
    color: 'transparent',
    itemTextColor: '#a1a1aa',
    itemTextColorHover: '#fafafa',
    itemTextColorActive: '#6366f1',
    itemColorActive: 'rgba(99, 102, 241, 0.1)',
    itemColorActiveHover: 'rgba(99, 102, 241, 0.15)',
  }
}

onMounted(() => {
  store.initialize()
})
</script>

<template>
  <NConfigProvider :theme="darkTheme" :theme-overrides="themeOverrides">
    <NMessageProvider>
      <NDialogProvider>
        <MainLayout />
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>

<style>
#app {
  width: 100%;
  height: 100%;
}
</style>
