<script setup lang="ts">
import { credentialReasonLabel, shouldShowCredentialReason } from '~/lib/credentialStatus'

/**
 * 凭据卡片共享骨架:头部(勾选 + 标题 + 徽章)与 reason 块各 handler 一致,
 * 中间的配额/详情区域由默认插槽承载各 handler 的差异部分。
 */
const props = withDefaults(defineProps<{
  title: string
  statuses?: string[] | string | null
  planType?: string | null
  reason?: string | null
  checked: boolean
}>(), {
  statuses: null,
  planType: null,
  reason: null,
})

const emit = defineEmits<{
  toggle: []
}>()
</script>

<template>
  <VCard color="surface-container" variant="flat">
    <VCardText class="stack-card-body">
      <div class="stack-card-top">
        <div class="d-flex align-start ga-3 stack-card-heading">
          <VCheckboxBtn
            :model-value="checked"
            @update:model-value="emit('toggle')"
          />
          <div class="stack-card-copy">
            <div class="stack-card-title">{{ title }}</div>
            <div class="stack-card-meta">
              <CredentialBadges :plan-type="props.planType ?? undefined" :statuses="statuses" />
            </div>
          </div>
        </div>
      </div>

      <slot />

      <div v-if="shouldShowCredentialReason(statuses) && reason" class="reason-block">
        <div class="reason-label">{{ credentialReasonLabel(statuses) }}</div>
        <div class="reason-value">{{ reason }}</div>
      </div>
    </VCardText>
  </VCard>
</template>

<style scoped>
.stack-card-heading {
  min-width: 0;
}
</style>
