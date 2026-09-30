/**
 * backend/main.ts — TinyJS backend for mdeditor
 *
 * Handles file-system operations (read / write) that the webview frontend
 * cannot do directly. Each handler is called via `window.tiny.api.call(name,
 * params)` from the React frontend.
 *
 * txiki.js provides the `tjs` global; see tjs.d.ts for types.
 */

declare const tjs: {
  readFile(path: string, encoding: 'utf8'): Promise<string>
  writeFile(path: string, data: string, encoding: 'utf8'): Promise<void>
}

interface ReadFileParams {
  path: string
}

interface WriteFileParams {
  path: string
  content: string
}

export const api: Record<string, (params: unknown) => Promise<unknown>> = {
  async readFile(raw: unknown) {
    const { path } = raw as ReadFileParams
    const content = await tjs.readFile(path, 'utf8')
    return { path, content }
  },

  async writeFile(raw: unknown) {
    const { path, content } = raw as WriteFileParams
    await tjs.writeFile(path, content, 'utf8')
    return {}
  },
}
