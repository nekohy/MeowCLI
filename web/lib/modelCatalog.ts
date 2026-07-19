export const DEFAULT_MODEL_CATALOG_URLS: Record<string, string> = {
  antigravity: 'https://cdn.jsdelivr.net/gh/nekohy/MeowCLI@master/models-list/antigravity.json',
  codex: 'https://cdn.jsdelivr.net/gh/nekohy/MeowCLI@master/models-list/codex.json',
  gemini: 'https://cdn.jsdelivr.net/gh/nekohy/MeowCLI@master/models-list/gemini-cli.json',
  'opencode-go': 'https://cdn.jsdelivr.net/gh/nekohy/MeowCLI@master/models-list/opencode-go.json',
}

export interface ModelCatalogItem {
  id: string
  name: string
  description: string
}

export function modelCatalogStorageKey(handlerKey: string) {
  return `meowcli_model_catalog_url_${handlerKey}`
}

function stringField(value: unknown) {
  return typeof value === 'string' ? value.trim() : ''
}

export function normalizeModelCatalog(value: unknown): ModelCatalogItem[] {
  if (!Array.isArray(value)) {
    throw new Error('模型列表必须是 JSON 数组')
  }

  const seen = new Set<string>()
  const models: ModelCatalogItem[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') {
      continue
    }

    const record = item as Record<string, unknown>
    const id = stringField(record.id)
    if (!id || seen.has(id)) {
      continue
    }

    seen.add(id)
    models.push({
      id,
      name: stringField(record.name) || id,
      description: stringField(record.description),
    })
  }

  return models
}
