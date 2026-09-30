import { useState, useRef, useCallback, useEffect } from 'react'
import type { Editor as TipTapEditor } from '@tiptap/core'
import Editor from './components/Editor'
import Toolbar from './components/Toolbar'
import { fromMarkdown, toMarkdown } from './lib/markdown'
import type { TipTapDoc } from './lib/markdown'
import {
  isTinyApp,
  tinyOpenFileDialog,
  tinySaveFileDialog,
  backendReadFile,
  backendWriteFile,
} from './lib/tinyapi'

export default function App() {
  const editorRef = useRef<TipTapEditor | null>(null)
  const [doc, setDoc] = useState<TipTapDoc | null>(null)
  const [filePath, setFilePath] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  const [wordCount, setWordCount] = useState(0)
  const [dirty, setDirty] = useState(false)
  const currentMarkdownRef = useRef('')

  // Keep markdown in sync for save operations
  const handleChange = useCallback((nextDoc: TipTapDoc) => {
    const md = toMarkdown(nextDoc)
    currentMarkdownRef.current = md
    setWordCount(md.replace(/\s+/g, '').length)
    setDirty(true)
  }, [])

  // Open file
  const handleOpen = useCallback(async () => {
    try {
      const path = await tinyOpenFileDialog()
      if (!path) return
      const result = await backendReadFile(path)
      const nextDoc = fromMarkdown(result.content)
      setDoc(nextDoc)
      setFilePath(result.path)
      setTitle(result.path.split('/').pop()?.replace(/\.md$/, '') ?? '')
      currentMarkdownRef.current = result.content
      setDirty(false)
    } catch (err) {
      console.warn('open file failed', err)
    }
  }, [])

  // Save to current path, or prompt if none
  const handleSave = useCallback(
    async (saveAs = false) => {
      try {
        let path = filePath
        if (!path || saveAs) {
          path = await tinySaveFileDialog()
          if (!path) return
          if (!path.endsWith('.md')) path += '.md'
        }
        await backendWriteFile(path, currentMarkdownRef.current)
        setFilePath(path)
        setTitle(path.split('/').pop()?.replace(/\.md$/, '') ?? '')
        setDirty(false)
      } catch (err) {
        console.warn('save file failed', err)
      }
    },
    [filePath],
  )

  // New file
  const handleNew = useCallback(() => {
    const blank: TipTapDoc = { type: 'doc', content: [{ type: 'paragraph' }] }
    setDoc(blank)
    setFilePath(null)
    setTitle('')
    currentMarkdownRef.current = ''
    setDirty(false)
  }, [])

  // Keyboard shortcuts
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey
      if (mod && e.key === 's') {
        e.preventDefault()
        void handleSave(e.shiftKey)
      }
      if (mod && e.key === 'o') {
        e.preventDefault()
        void handleOpen()
      }
      if (mod && e.key === 'n') {
        e.preventDefault()
        handleNew()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [handleSave, handleOpen, handleNew])

  const inTiny = isTinyApp()
  const displayTitle = title || '未命名'

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-left">
          {inTiny && (
            <>
              <button className="header-btn" onClick={handleNew} title="新建 (Mod+N)">
                新建
              </button>
              <button className="header-btn" onClick={handleOpen} title="打开 (Mod+O)">
                打开
              </button>
              <button
                className="header-btn"
                onClick={() => handleSave(false)}
                title="保存 (Mod+S)"
              >
                保存{dirty ? ' *' : ''}
              </button>
              <button
                className="header-btn"
                onClick={() => handleSave(true)}
                title="另存为 (Mod+Shift+S)"
              >
                另存为
              </button>
            </>
          )}
        </div>
        <div className="header-center">
          <span className="file-title" title={filePath ?? ''}>
            {displayTitle}
            {dirty ? ' *' : ''}
          </span>
        </div>
        <div className="header-right">
          <span className="word-count">字数 {wordCount}</span>
        </div>
      </header>

      <div className="editor-area">
        <Toolbar editor={editorRef.current} />
        <Editor
          ref={editorRef}
          initialDoc={doc}
          onChange={handleChange}
          placeholder="开始输入 Markdown…（支持 CommonMark 格式）"
        />
      </div>
    </div>
  )
}
