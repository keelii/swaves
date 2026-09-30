import { useEffect, useImperativeHandle, forwardRef } from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import type { Editor as TipTapEditor } from '@tiptap/core'
import type { TipTapDoc } from '../lib/markdown'

export type EditorRef = TipTapEditor | null

interface EditorProps {
  initialDoc?: TipTapDoc | null
  placeholder?: string
  onChange?: (doc: TipTapDoc) => void
}

const Editor = forwardRef<EditorRef, EditorProps>(function Editor(
  { initialDoc, placeholder = '开始输入 Markdown…', onChange },
  ref,
) {
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3, 4, 5, 6] },
      }),
      Placeholder.configure({ placeholder }),
    ],
    content: initialDoc ?? { type: 'doc', content: [{ type: 'paragraph' }] },
    autofocus: true,
    onUpdate({ editor: e }) {
      if (onChange) {
        onChange(e.getJSON() as TipTapDoc)
      }
    },
  })

  useImperativeHandle(ref, () => editor, [editor])

  // Reload content when initialDoc changes (e.g. opening a new file)
  useEffect(() => {
    if (!editor || !initialDoc) return
    const current = JSON.stringify(editor.getJSON())
    const next = JSON.stringify(initialDoc)
    if (current !== next) {
      editor.commands.setContent(initialDoc, { emitUpdate: false })
    }
  }, [editor, initialDoc])

  return (
    <div className="editor-wrapper">
      <EditorContent editor={editor} className="editor-content" />
    </div>
  )
})

export default Editor
