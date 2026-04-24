import { adminApi } from '~/composables/useAdminApi'
import type { ImportJobSnapshot } from '~/types/admin'

const IMPORT_JOB_POLL_MS = 1200
let importJobPollTimer: number | undefined

export function useImportJobs() {
  const jobs = useState<ImportJobSnapshot[]>('admin-jobs', () => [])
  const dismissed = useState<string[]>('admin-job-dismissed', () => [])
  const loading = useState<boolean>('admin-jobs-loading', () => false)

  const activeJobs = computed(() => jobs.value.filter((job) => !job.done))
  const visibleJobs = computed(() => jobs.value.filter((job) => !dismissed.value.includes(job.id)))

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
      jobs.value = (response.data || []).sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))
    } finally {
      loading.value = false
    }
  }

  function add(job: ImportJobSnapshot) {
    merge([job])
    dismissed.value = dismissed.value.filter((id) => id !== job.id)
  }

  function dismiss(id: string) {
    if (!dismissed.value.includes(id)) {
      dismissed.value = [...dismissed.value, id]
    }
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
