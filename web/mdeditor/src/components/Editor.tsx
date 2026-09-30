import { useEffect, useRef, useImperativeHandle, forwardRef } from 'react'

interface EditorProps {
  initialMarkdown?: string
  placeholder?: string
  commandsRoot?: HTMLElement | null
  onChange?: (markdown: string) => void
}

const Editor = forwardRef<SEditorInstance | null, EditorProps>(function Editor(
  { initialMarkdown, placeholder, commandsRoot, onChange },
  ref,
) {
  const mountRef = useRef<HTMLDivElement>(null)
  const instanceRef = useRef<SEditorInstance | null>(null)

  useImperativeHandle(ref, () => instanceRef.current as SEditorInstance, [])

  useEffect(() => {
    const el = mountRef.current
    if (!el) return

    let instance: SEditorInstance | null = null

    instance = window.SEditor.init({
      mount: el,
      initialMarkdown: initialMarkdown ?? '',
      placeholder: placeholder ?? '开始输入 Markdown…',
      commandsRoot: commandsRoot ?? document,
      onChange,
    })
    instanceRef.current = instance

    return () => {
      instance?.destroy()
      instanceRef.current = null
    }
    // Only run on mount/unmount — content changes go through setMarkdown
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className="editor-wrapper">
      <div ref={mountRef} className="editor-content" />
    </div>
  )
})

export default Editor
