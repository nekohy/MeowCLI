<script setup lang="ts">
import type { ImportJobSnapshot } from '~/types/admin'

const admin = useAdminApp()
const importJobs = useImportJobs()

const visibleJobs = computed(() => importJobs.visibleJobs.value)
const hasVisibleJobs = computed(() => visibleJobs.value.length > 0)
const runningCount = computed(() => visibleJobs.value.filter((job) => !job.done).length)

function handlerLabel(handler: string) {
  return admin.handlers.value.find((item) => item.key === handler)?.label || handler
}

function progressValue(job: ImportJobSnapshot) {
  if (!job.total) {
    return 100
  }
  return Math.max(0, Math.min(100, Math.round((job.processed / job.total) * 100)))
}

function closeAll() {
  visibleJobs.value.forEach((job) => importJobs.dismiss(job.id))
}

async function refreshAndPoll() {
  if (!admin.token.value) {
    return
  }
  await importJobs.refresh(admin.token.value)
  if (importJobs.activeJobs.value.length > 0) {
    importJobs.ensurePolling(admin.token.value)
  }
}

watch(
  () => admin.token.value,
  (token) => {
    if (token) {
      void refreshAndPoll()
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  importJobs.clearPolling()
})
</script>

<template>
  <Transition name="import-dock">
    <VCard
      v-if="hasVisibleJobs"
      class="import-job-dock"
      color="surface-container-high"
      variant="flat"
      rounded="xl"
    >
      <VCardText class="import-job-dock__body">
        <div class="import-job-dock__header">
          <div class="import-job-dock__title">
            <VAvatar size="34" color="primary-container" rounded="lg">
              <VIcon icon="mdi-cloud-upload-outline" color="primary" size="19" />
            </VAvatar>
            <div>
              <div class="text-subtitle-2 font-weight-bold">导入任务</div>
              <div class="text-caption text-medium-emphasis">
                {{ runningCount > 0 ? `${runningCount} 个任务运行中` : '任务已完成' }}
              </div>
            </div>
          </div>
          <VBtn
            icon="mdi-close"
            variant="text"
            density="comfortable"
            size="small"
            aria-label="关闭导入任务面板"
            @click="closeAll"
          />
        </div>

        <div class="import-job-list">
          <VSheet
            v-for="job in visibleJobs"
            :key="job.id"
            class="import-job-row"
            color="surface-container"
            rounded="lg"
          >
            <div class="import-job-row__top">
              <div class="import-job-row__copy">
                <div class="text-body-2 font-weight-medium">{{ handlerLabel(job.handler) }}</div>
                <div class="text-caption text-medium-emphasis">
                  {{ job.processed }} / {{ job.total }}
                </div>
              </div>
              <VBtn
                icon="mdi-close"
                variant="text"
                density="compact"
                size="x-small"
                aria-label="关闭该导入任务"
                @click="importJobs.dismiss(job.id)"
              />
            </div>

            <VProgressLinear
              :model-value="progressValue(job)"
              color="primary"
              height="8"
              rounded
            />
          </VSheet>
        </div>
      </VCardText>
    </VCard>
  </Transition>
</template>

<style scoped>
.import-job-dock {
  position: fixed;
  right: max(16px, env(safe-area-inset-right));
  bottom: max(16px, env(safe-area-inset-bottom));
  z-index: 2400;
  width: min(420px, calc(100vw - 32px));
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.72);
  box-shadow: var(--v-shadow-4);
}

.import-job-dock__body {
  display: grid;
  gap: 8px;
  padding: 12px 14px;
}

.import-job-dock__header,
.import-job-row__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.import-job-dock__title {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.import-job-list {
  display: grid;
  gap: 8px;
  max-height: min(48vh, 420px);
  overflow: auto;
}

.import-job-row {
  display: grid;
  gap: 8px;
  padding: 10px;
}

.import-job-row__copy {
  min-width: 0;
}

.import-dock-enter-active,
.import-dock-leave-active {
  transition: opacity 180ms ease, transform 180ms ease;
}

.import-dock-enter-from,
.import-dock-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.98);
}
</style>
