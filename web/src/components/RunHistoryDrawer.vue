<script setup>
import { computed, h } from 'vue'
import { NDrawer, NDrawerContent, NDataTable, NTag } from 'naive-ui'

const props = defineProps({
  show: Boolean,
  history: { type: Array, default: () => [] },
})

defineEmits(['update:show'])

const columns = [
  { title: 'ID', key: 'id', width: 100, ellipsis: { tooltip: true } },
  {
    title: '模式',
    key: 'mode',
    width: 110,
    render: (row) => {
      const map = { standard: '标准', skip_backup: '跳过备份', code_only: '仅代码' }
      return map[row.mode] || row.mode
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row) => {
      const type = row.status === 'success' ? 'success' : row.status === 'failed' ? 'error' : 'warning'
      return h(NTag, { size: 'small', type, bordered: false }, { default: () => row.status })
    },
  },
  {
    title: '时间',
    key: 'created_at',
    render: (row) => (row.created_at ? new Date(row.created_at).toLocaleString('zh-CN') : '—'),
  },
]

const rows = computed(() => props.history)
</script>

<template>
  <NDrawer :show="show" :width="520" @update:show="$emit('update:show', $event)">
    <NDrawerContent title="发布历史" closable>
      <NDataTable :columns="columns" :data="rows" :bordered="false" size="small" />
    </NDrawerContent>
  </NDrawer>
</template>
