/**
 * Ambient declarations for @swaves/seditor loaded as a global IIFE script.
 * seditor is built with: esbuild --format=iife --global-name=SEditor
 * and injected via <script src="/seditor.js"> in index.html.
 */

interface SEditorInitOptions {
  /** Mount element or CSS selector (required). */
  mount: HTMLElement | string
  /** Hidden textarea to keep in sync with markdown content. */
  textarea?: HTMLElement | string
  /** Initial markdown string. */
  initialMarkdown?: string
  /** Placeholder text when the document is empty. */
  placeholder?: string
  /** Called on every document change with the serialized markdown. */
  onChange?: (markdown: string) => void
  /** Root element for data-seditor-command button binding. Defaults to document. */
  commandsRoot?: HTMLElement | Document | string
  /** Whether to render raw block previews (math / iframe). Defaults to false. */
  rawBlockPreview?: boolean
  /** Set to false to skip command button binding. */
  bindCommands?: boolean
}

interface SEditorInstance {
  /** Get the current document serialized as CommonMark markdown. */
  getMarkdown(): string
  /** Replace the document with new markdown content. */
  setMarkdown(markdown: string): void
  /** Focus the editor. */
  focus(): void
  /** Refresh mermaid diagram previews (if any). */
  refreshMermaidPreviews(): void
  /** Destroy the editor and clean up. */
  destroy(): void
}

interface Window {
  SEditor: {
    init(options: SEditorInitOptions): SEditorInstance
  }
}
