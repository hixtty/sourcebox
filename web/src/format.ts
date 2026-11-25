export function formatBytes(bytes?: number | null): string {
  if (!bytes || bytes <= 0) {
    return '—'
  }
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  const digits = value >= 100 || index === 0 ? 0 : 1
  return `${value.toFixed(digits)} ${units[index]}`
}

export function timeAgo(iso?: string | null): string {
  if (!iso) {
    return '从未'
  }
  const stamp = new Date(iso).getTime()
  if (Number.isNaN(stamp)) {
    return '从未'
  }
  const seconds = Math.max(0, Date.now() - stamp) / 1000
  if (seconds < 45) {
    return '刚刚'
  }
  if (seconds < 3600) {
    return `${Math.round(seconds / 60)} 分钟前`
  }
  if (seconds < 86400) {
    return `${Math.round(seconds / 3600)} 小时前`
  }
  if (seconds < 60 * 86400) {
    return `${Math.round(seconds / 86400)} 天前`
  }
  return new Date(iso).toLocaleDateString()
}

export function formatDate(iso?: string | null): string {
  if (!iso) {
    return '—'
  }
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) {
    return '—'
  }
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // Fall through to the legacy path.
  }
  try {
    const input = document.createElement('textarea')
    input.value = text
    input.setAttribute('readonly', '')
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    const ok = document.execCommand('copy')
    input.remove()
    return ok
  } catch {
    return false
  }
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message
  }
  return '出了点问题，请重试。'
}

