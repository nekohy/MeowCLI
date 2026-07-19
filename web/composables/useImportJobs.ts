import { adminApi, ApiError } from '~/composables/useAdminApi'
import type { ImportJobSnapshot, ImportJobStartResponse } from '~/types/admin'

const IMPORT_JOB_POLL_MS = 1000
let importJobPollTimer: number | undefined
let importJobPollToken = ''

export function newlyCompletedImportJobs(
  jobs: ImportJobSnapshot[],
  previousJobs: ImportJobSnapshot[] = [],
) {
  const previousStatuses = new Map(previousJobs.map((job) => [job.id, job.status]))
  return jobs.filter((job) => (
    job.status === 'completed' && previousStatuses.get(job.id) !== 'completed'
  ))
}

export function useImportJobs() {
  const jobs = useState<ImportJobSnapshot[]>('admin-jobs', () => [])
  const dismissed = useState<string[]>('admin-job-dismissed', () => [])

  const activeJobs = computed(() => jobs.value.filter((job) => job.status !== 'completed'))
  const visibleJobs = computed(() => jobs.value.filter((job) => !dismissed.value.includes(job.id)))

  async function acknowledgeCompletedJobs(token: string, completedJobs: ImportJobSnapshot[]) {
    const targets = completedJobs.filter((job) => job.status === 'completed')
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
    const response = await adminApi.listJobs(token)
    const nextJobs = response.data.sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))
    jobs.value = nextJobs
    await acknowledgeCompletedJobs(token, nextJobs.filter((job) => dismissed.value.includes(job.id)))
  }

  function add(job: ImportJobStartResponse) {
    merge([{
      ...job,
      processed: 0,
      succeeded: 0,
      error: [],
      created_at: '',
      updated_at: '',
    }])
    dismissed.value = dismissed.value.filter((id) => id !== job.id)
  }

  async function dismiss(job: ImportJobSnapshot, token = '') {
    const { id } = job
    if (!dismissed.value.includes(id)) {
      dismissed.value = [...dismissed.value, id]
    }

    if (!token || job.status !== 'completed') {
      return
    }

    await acknowledgeCompletedJobs(token, [job])
  }

  function clearPolling() {
    if (importJobPollTimer && import.meta.client) {
      window.clearTimeout(importJobPollTimer)
    }
    importJobPollTimer = undefined
    importJobPollToken = ''
  }

  async function runPollTick() {
    // 后台标签页不轮询，恢复可见时下一拍自动继续
    if (document.visibilityState === 'visible') {
      await refresh(importJobPollToken).catch(() => undefined)
    }
    // await 期间可能已被 clearPolling（登出/组件卸载）:token 清空即停止，
    // 否则旧链会每秒空转 refresh('') 直到页面关闭
    if (!importJobPollToken) {
      importJobPollTimer = undefined
      return
    }
    // 等上一次请求 settle 后再排下一拍，避免请求堆积与乱序回写；
    // 判空也放在 refresh 之后，最后一个任务完成时立即停止
    if (activeJobs.value.length === 0) {
      clearPolling()
      return
    }
    importJobPollTimer = window.setTimeout(() => void runPollTick(), IMPORT_JOB_POLL_MS)
  }

  function ensurePolling(token: string) {
    if (!import.meta.client || !token) {
      return
    }
    // 登录态变化后更新轮询使用的 token，已存在的定时器继续复用
    importJobPollToken = token
    if (importJobPollTimer) {
      return
    }
    importJobPollTimer = window.setTimeout(() => void runPollTick(), IMPORT_JOB_POLL_MS)
  }

  return {
    activeJobs,
    add,
    clearPolling,
    dismiss,
    ensurePolling,
    jobs,
    refresh,
    visibleJobs,
  }
}
