<script setup lang="ts">
import MaintenanceRow from '~/components/settings/MaintenanceRow.vue'

defineProps<{
  quotaBusy?: boolean
  logsBusy?: boolean
}>()

const emit = defineEmits<{
  refresh: []
  clear: []
}>()
</script>

<template>
  <MaintenanceRow
    label="刷新额度"
    description="立即刷新该渠道所有凭据的额度"
    action-text="立刻刷新"
    icon="mdi-refresh"
    :loading="quotaBusy"
    :disabled="logsBusy"
    @activate="emit('refresh')"
  />
  <MaintenanceRow
    label="请求日志"
    description="立刻清空该渠道请求日志"
    action-text="清空请求日志"
    icon="mdi-delete-outline"
    variant="danger"
    :loading="logsBusy"
    :disabled="quotaBusy"
    @activate="emit('clear')"
  />
</template>
