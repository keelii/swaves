import { useState, useRef, useCallback, useEffect } from 'react'
import Editor from './components/Editor'
import Toolbar from './components/Toolbar'
import {
  isTinyApp,
  tinyOpenFileDialog,
  tinySaveFileDialog,
  backendReadFile,
  backendWriteFile,
} from './lib/tinyapi'

export default function App() {
  const editorRef = useRef<SEditorInstance | null>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)
  const [filePath, setFilePath] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  const [wordCount, setWordCount] = useState(0)
  const [dirty, setDirty] = useState(false)
  const [initialMarkdown, setInitialMarkdown] = useState('')

  const handleChange = useCallback((markdown: string) => {
    setWordCount(markdown.replace(/\s+/g, '').length)
    setDirty(true)
  }, [])

  const handleOpen = useCallback(async () => {
    try {
      const path = await tinyOpenFileDialog()
      if (!path) return
      const result = await backendReadFile(path)
      if (editorRef.current) {
        editorRef.current.setMarkdown(result.content)
      } else {
        setInitialMarkdown(result.content)
      }
      setFilePath(result.path)
      setTitle(result.path.split('/').pop()?.replace(/\.md$/, '') ?? '')
      setWordCount(result.content.replace(/\s+/g, '').length)
      setDirty(false)
    } catch (err) {
      console.warn('open file failed', err)
    }
  }, [])

  const handleSave = useCallback(
    async (saveAs = false) => {
      const instance = editorRef.current
      if (!instance) return
      try {
        let path = filePath
        if (!path || saveAs) {
          path = await tinySaveFileDialog()
          if (!path) return
          if (!path.endsWith('.md')) path += '.md'
        }
        await backendWriteFile(path, instance.getMarkdown())
        setFilePath(path)
        setTitle(path.split('/').pop()?.replace(/\.md$/, '') ?? '')
        setDirty(false)
      } catch (err) {
        console.warn('save file failed', err)
      }
    },
    [filePath],
  )

  const handleNew = useCallback(() => {
    if (editorRef.current) {
      editorRef.current.setMarkdown('')
    } else {
      setInitialMarkdown('')
    }
    setFilePath(null)
    setTitle('')
    setWordCount(0)
    setDirty(false)
  }, [])

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
        <div ref={toolbarRef}>
          <Toolbar />
        </div>
        <Editor
          ref={editorRef}
          initialMarkdown={initialMarkdown}
          commandsRoot={toolbarRef}
          onChange={handleChange}
          placeholder="开始输入 Markdown…（支持 CommonMark 格式）"
        />
      </div>
    </div>
  )
}
