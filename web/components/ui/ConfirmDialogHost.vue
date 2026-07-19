<script setup lang="ts">
/**
 * useConfirmDialog 的共享宿主:统一渲染确认弹窗,
 * 确认按钮文案与色调由 confirm.show() 的调用方决定,不在此处硬编码。
 */
defineProps<{
  confirm: ReturnType<typeof useConfirmDialog>
  actionBusy: boolean
  description?: string
  icon?: string
}>()
</script>

<template>
  <ModalDialog
    :open="confirm.open.value"
    :title="confirm.title.value"
    :description="description"
    :icon="icon"
    @close="confirm.close()"
  >
    <p class="text-body-1">{{ confirm.message.value }}</p>
    <template #footer>
      <AdminButton variant="ghost" :disabled="actionBusy" @click="confirm.close()">取消</AdminButton>
      <AdminButton
        :variant="confirm.variant.value"
        :loading="actionBusy"
        @click="confirm.submit()"
      >
        {{ confirm.text.value }}
      </AdminButton>
    </template>
  </ModalDialog>
</template>
