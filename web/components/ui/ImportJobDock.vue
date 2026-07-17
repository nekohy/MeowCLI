<script setup lang="ts">
import { newlyCompletedImportJobs } from '~/composables/useImportJobs'
import type { ImportJobSnapshot } from '~/types/admin'

const admin = useAdminApp()
const importJobs = useImportJobs()

const visibleJobs = computed(() => importJobs.visibleJobs.value)
const hasVisibleJobs = computed(() => visibleJobs.value.length > 0)
const runningCount = computed(() => visibleJobs.value.filter((job) => job.status !== 'completed').length)
const errorDialogOpen = ref(false)
const selectedJob = ref<ImportJobSnapshot | null>(null)
const selectedJobErrors = computed(() => errorEntries(selectedJob.value))

function handlerLabel(handler: string) {
  return admin.handlers.value.find((item) => item.key === handler)?.label || handler
}

function progressValue(job: ImportJobSnapshot) {
  if (!job.total) {
    return 100
  }
  return Math.max(0, Math.min(100, Math.round((job.processed / job.total) * 100)))
}

function jobProgressText(job: ImportJobSnapshot) {
  if (job.status === 'completed') {
    return `成功 ${job.succeeded} / ${job.total}`
  }
  return `已处理 ${job.processed} / ${job.total}`
}

function errorEntries(job: ImportJobSnapshot | null) {
  if (!job?.error?.length) {
    return []
  }
  return job.error.flatMap((item) => Object.entries(item).map(([input, message]) => ({ input, message })))
}

function showErrors(job: ImportJobSnapshot) {
  selectedJob.value = job
  errorDialogOpen.value = true
}

function closeJob(job: ImportJobSnapshot) {
  void importJobs.dismiss(job, admin.token.value)
}

function closeAll() {
  visibleJobs.value.forEach((job) => void importJobs.dismiss(job, admin.token.value))
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

watch(
  () => importJobs.jobs.value,
  (jobs, previousJobs) => {
    if (!admin.token.value || newlyCompletedImportJobs(jobs, previousJobs).length === 0) {
      return
    }
    void admin.loadOverview(admin.token.value, true)
  },
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
            tabindex="0"
            role="button"
            :aria-label="`${handlerLabel(job.handler)}导入任务详情`"
            @click="showErrors(job)"
            @keydown.enter.prevent="showErrors(job)"
            @keydown.space.prevent="showErrors(job)"
          >
            <div class="import-job-row__top">
              <div class="import-job-row__copy">
                <div class="text-body-2 font-weight-medium">{{ handlerLabel(job.handler) }}</div>
                <div class="text-caption text-medium-emphasis">
                  {{ jobProgressText(job) }}
                </div>
              </div>
              <VBtn
                icon="mdi-close"
                variant="text"
                density="compact"
                size="x-small"
                aria-label="关闭该导入任务"
                @click.stop="closeJob(job)"
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

  <ModalDialog
    :open="errorDialogOpen"
    :title="selectedJob ? `${handlerLabel(selectedJob.handler)} 导入错误` : '导入错误'"
    :description="selectedJob ? `成功 ${selectedJob.succeeded} / ${selectedJob.total}` : undefined"
    icon="mdi-alert-circle-outline"
    max-width="620"
    @close="errorDialogOpen = false"
  >
    <div v-if="selectedJobErrors.length" class="import-error-list">
      <div
        v-for="(item, idx) in selectedJobErrors"
        :key="`${item.input}-${idx}`"
        class="import-error-item"
      >
        <span class="import-error-copy">
          <span>{{ item.input }}</span>
          <small>{{ item.message }}</small>
        </span>
      </div>
    </div>
    <div v-else class="text-center text-medium-emphasis py-4">
      暂无错误
    </div>
    <div class="import-error-footer">
      <VBtn variant="text" @click="errorDialogOpen = false">关闭</VBtn>
    </div>
  </ModalDialog>
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
  cursor: pointer;
  transition: background 0.15s, box-shadow 0.15s;
}

.import-job-row:hover,
.import-job-row:focus-visible {
  background: rgba(var(--v-theme-on-surface), 0.08) !important;
}

.import-job-row:focus-visible {
  outline: 2px solid rgba(var(--v-theme-primary), 0.55);
  outline-offset: 2px;
}

.import-job-row__copy {
  min-width: 0;
}

.import-error-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.import-error-item {
  min-height: 58px;
  padding: 10px 12px;
  border-radius: 10px;
  background: rgba(var(--v-theme-on-surface), 0.04);
}

.import-error-copy {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.import-error-copy > span {
  color: rgba(var(--v-theme-on-surface), 0.91);
  font-size: 0.875rem;
  font-weight: 600;
  line-height: 1.25;
  overflow-wrap: anywhere;
}

.import-error-copy > small {
  color: rgba(var(--v-theme-on-surface), 0.58);
  font-size: 0.75rem;
  font-weight: 450;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.import-error-footer {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
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
