import type { Editor } from '@tiptap/core'

interface ToolbarProps {
  editor: Editor | null
}

interface ToolbarButton {
  label: string
  title: string
  action: (editor: Editor) => void
  isActive?: (editor: Editor) => boolean
}

const BUTTONS: ToolbarButton[] = [
  {
    label: 'B',
    title: '加粗 (Mod+B)',
    action: e => e.chain().focus().toggleBold().run(),
    isActive: e => e.isActive('bold'),
  },
  {
    label: 'I',
    title: '斜体 (Mod+I)',
    action: e => e.chain().focus().toggleItalic().run(),
    isActive: e => e.isActive('italic'),
  },
  {
    label: 'S',
    title: '删除线',
    action: e => e.chain().focus().toggleStrike().run(),
    isActive: e => e.isActive('strike'),
  },
  {
    label: '`',
    title: '行内代码',
    action: e => e.chain().focus().toggleCode().run(),
    isActive: e => e.isActive('code'),
  },
  { label: '|', title: '', action: () => {} }, // divider
  {
    label: 'H1',
    title: '一级标题',
    action: e => e.chain().focus().toggleHeading({ level: 1 }).run(),
    isActive: e => e.isActive('heading', { level: 1 }),
  },
  {
    label: 'H2',
    title: '二级标题',
    action: e => e.chain().focus().toggleHeading({ level: 2 }).run(),
    isActive: e => e.isActive('heading', { level: 2 }),
  },
  {
    label: 'H3',
    title: '三级标题',
    action: e => e.chain().focus().toggleHeading({ level: 3 }).run(),
    isActive: e => e.isActive('heading', { level: 3 }),
  },
  { label: '|', title: '', action: () => {} },
  {
    label: '≡',
    title: '无序列表',
    action: e => e.chain().focus().toggleBulletList().run(),
    isActive: e => e.isActive('bulletList'),
  },
  {
    label: '1.',
    title: '有序列表',
    action: e => e.chain().focus().toggleOrderedList().run(),
    isActive: e => e.isActive('orderedList'),
  },
  {
    label: '❝',
    title: '引用',
    action: e => e.chain().focus().toggleBlockquote().run(),
    isActive: e => e.isActive('blockquote'),
  },
  {
    label: '⌨',
    title: '代码块',
    action: e => e.chain().focus().toggleCodeBlock().run(),
    isActive: e => e.isActive('codeBlock'),
  },
  { label: '|', title: '', action: () => {} },
  {
    label: '⟵',
    title: '撤销 (Mod+Z)',
    action: e => e.chain().focus().undo().run(),
  },
  {
    label: '⟶',
    title: '重做 (Mod+Shift+Z)',
    action: e => e.chain().focus().redo().run(),
  },
]

export default function Toolbar({ editor }: ToolbarProps) {
  if (!editor) return null

  return (
    <div className="toolbar" role="toolbar" aria-label="格式化工具栏">
      {BUTTONS.map((btn, i) => {
        if (btn.label === '|') {
          return <span key={i} className="toolbar-divider" aria-hidden="true" />
        }
        const active = btn.isActive ? btn.isActive(editor) : false
        return (
          <button
            key={i}
            type="button"
            title={btn.title}
            aria-label={btn.title}
            aria-pressed={active}
            className={`toolbar-btn${active ? ' active' : ''}`}
            onMouseDown={e => {
              e.preventDefault()
              btn.action(editor)
            }}
          >
            {btn.label}
          </button>
        )
      })}
    </div>
  )
}
