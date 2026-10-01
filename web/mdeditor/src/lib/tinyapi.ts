/**
 * tinyapi.ts
 *
 * Thin wrapper around `window.tiny` (injected by the TinyJS launcher).
 * Falls back gracefully in a plain browser for `npm run dev`.
 */

declare global {
  interface Window {
    tiny?: {
      api: {
        call(method: string, params?: unknown): Promise<unknown>
        on(event: string, fn: (data: unknown) => void): void
      }
      dialog: {
        openFile(opts?: { types?: string[] }): Promise<string | null>
        saveFile(opts?: { types?: string[] }): Promise<string | null>
      }
    }
  }
}

export function isTinyApp(): boolean {
  return typeof window !== 'undefined' && typeof window.tiny !== 'undefined'
}

export async function tinyCall<T = unknown>(
  method: string,
  params?: unknown,
): Promise<T> {
  if (!isTinyApp()) throw new Error('tiny API not available (browser-only mode)')
  return (await window.tiny!.api.call(method, params)) as T
}

export async function tinyOpenFileDialog(): Promise<string | null> {
  if (!isTinyApp()) return null
  return window.tiny!.dialog.openFile({ types: ['md', 'txt', 'markdown'] })
}

export async function tinySaveFileDialog(): Promise<string | null> {
  if (!isTinyApp()) return null
  return window.tiny!.dialog.saveFile({ types: ['md'] })
}

// ── Typed backend calls ──────────────────────────────────────────────────────

export interface FileResult {
  path: string
  content: string
}

export async function backendReadFile(path: string): Promise<FileResult> {
  return tinyCall<FileResult>('readFile', { path })
}

export async function backendWriteFile(path: string, content: string): Promise<void> {
  return tinyCall<void>('writeFile', { path, content })
}
