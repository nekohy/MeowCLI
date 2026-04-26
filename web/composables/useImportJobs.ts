import { adminApi, ApiError } from '~/composables/useAdminApi'
import type { ImportJobSnapshot } from '~/types/admin'

const IMPORT_JOB_POLL_MS = 5000
let importJobPollTimer: number | undefined

export function useImportJobs() {
  const jobs = useState<ImportJobSnapshot[]>('admin-jobs', () => [])
  const dismissed = useState<string[]>('admin-job-dismissed', () => [])
  const loading = useState<boolean>('admin-jobs-loading', () => false)

  const activeJobs = computed(() => jobs.value.filter((job) => !job.done))
  const visibleJobs = computed(() => jobs.value.filter((job) => !dismissed.value.includes(job.id)))

  async function acknowledgeCompletedJobs(token: string, completedJobs: ImportJobSnapshot[]) {
    const targets = completedJobs.filter((job) => job.done)
    if (!token || targets.length === 0) {
      return
    }

    const results = await Promise.allSettled(targets.map(async (job) => {
      try {
        await adminApi.acknowledgeJob(token, job.id)
      } catch (error) {
        if (!(error instanceof ApiError) || error.status !== 404) {
          throw error
        }
      }
      return job.id
    }))

    const acknowledged = results
      .filter((result): result is PromiseFulfilledResult<string> => result.status === 'fulfilled')
      .map((result) => result.value)

    if (acknowledged.length === 0) {
      return
    }

    const acknowledgedSet = new Set(acknowledged)
    jobs.value = jobs.value.filter((job) => !acknowledgedSet.has(job.id))
    dismissed.value = dismissed.value.filter((id) => !acknowledgedSet.has(id))
  }

  function merge(nextJobs: ImportJobSnapshot[]) {
    const byID = new Map(jobs.value.map((job) => [job.id, job]))
    nextJobs.forEach((job) => byID.set(job.id, job))
    jobs.value = [...byID.values()].sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))
  }

  async function refresh(token: string) {
    if (!token) {
      return
    }
    loading.value = true
    try {
      const response = await adminApi.listJobs(token)
      const nextJobs = response.data.sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))
      jobs.value = nextJobs
      await acknowledgeCompletedJobs(token, nextJobs.filter((job) => dismissed.value.includes(job.id)))
    } finally {
      loading.value = false
    }
  }

  function add(job: ImportJobSnapshot) {
    merge([job])
    dismissed.value = dismissed.value.filter((id) => id !== job.id)
  }

  async function dismiss(job: ImportJobSnapshot, token = '') {
    const { id } = job
    if (!dismissed.value.includes(id)) {
      dismissed.value = [...dismissed.value, id]
    }

    if (!token || !job.done) {
      return
    }

    await acknowledgeCompletedJobs(token, [job])
  }

  function clearPolling() {
    if (importJobPollTimer && import.meta.client) {
      window.clearInterval(importJobPollTimer)
    }
    importJobPollTimer = undefined
  }

  function ensurePolling(token: string) {
    if (!import.meta.client || importJobPollTimer || !token) {
      return
    }
    importJobPollTimer = window.setInterval(() => {
      void refresh(token).catch(() => undefined)
      if (activeJobs.value.length === 0) {
        clearPolling()
      }
    }, IMPORT_JOB_POLL_MS)
  }

  return {
    activeJobs,
    add,
    clearPolling,
    dismiss,
    ensurePolling,
    jobs,
    loading,
    refresh,
    visibleJobs,
  }
}
