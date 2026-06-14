<script setup>
import { NConfigProvider, NMessageProvider, darkTheme } from 'naive-ui'
import MainView from './views/MainView.vue'

const themeOverrides = {
  common: {
    primaryColor: '#3b82f6',
    primaryColorHover: '#2563eb',
    primaryColorPressed: '#1d4ed8',
    bodyColor: '#070b10',
    cardColor: 'rgba(18, 24, 38, 0.72)',
    borderColor: 'rgba(255,255,255,0.08)',
  },
}
</script>

<template>
  <NConfigProvider :theme="darkTheme" :theme-overrides="themeOverrides">
    <NMessageProvider>
      <MainView />
    </NMessageProvider>
  </NConfigProvider>
</template>
