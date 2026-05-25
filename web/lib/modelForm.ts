export function resolveCreateModelHandler(
  handlerFilter: string,
  activeHandlerKey: string | undefined,
  handlerKeys: string[],
) {
  const available = new Set(handlerKeys)
  if (handlerFilter !== 'all' && available.has(handlerFilter)) {
    return handlerFilter
  }
  if (activeHandlerKey && available.has(activeHandlerKey)) {
    return activeHandlerKey
  }
  return handlerKeys[0] || 'codex'
}

