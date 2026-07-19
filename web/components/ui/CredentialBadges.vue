<script setup lang="ts">
import { planTypeText, statusText, toneForStatus } from '~/lib/admin'
import { credentialStatusBadges, credentialStatusIcon } from '~/lib/credentialStatus'

const props = withDefaults(defineProps<{
  planType?: string
  statuses?: string[] | string | null
}>(), {
  planType: undefined,
  statuses: null,
})

const badges = computed(() => credentialStatusBadges(props.statuses))
</script>

<template>
  <AdminBadge v-if="planType !== undefined" tone="secondary" subtle icon="mdi-star-circle-outline">
    {{ planTypeText(planType) }}
  </AdminBadge>
  <AdminBadge
    v-for="status in badges"
    :key="status"
    :tone="toneForStatus(status)"
    subtle
    :icon="credentialStatusIcon(status)"
  >
    {{ statusText(status) }}
  </AdminBadge>
</template>
