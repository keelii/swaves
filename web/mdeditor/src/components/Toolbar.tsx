// Toolbar uses `data-seditor-command` attributes so SEditor can bind
// and refresh active-state automatically via `commandsRoot`.
// No TipTap or editor instance reference needed here.

interface ToolbarItem {
  command?: string
  label: string
  title: string
}

const ITEMS: ToolbarItem[] = [
  { command: 'bold',         label: 'B',  title: '加粗 (Mod+B)' },
  { command: 'italic',       label: 'I',  title: '斜体 (Mod+I)' },
  { command: 'inline_code',  label: '`',  title: '行内代码' },
  { label: '|', title: '' },
  { command: 'link',         label: '🔗', title: '链接' },
  { label: '|', title: '' },
  { command: 'blockquote',   label: '❝',  title: '引用' },
  { command: 'bullet_list',  label: '≡',  title: '无序列表' },
  { command: 'ordered_list', label: '1.', title: '有序列表' },
  { label: '|', title: '' },
  { command: 'undo',         label: '⟵', title: '撤销 (Mod+Z)' },
  { command: 'redo',         label: '⟶', title: '重做 (Mod+Shift+Z)' },
]

export default function Toolbar() {
  return (
    <div className="toolbar" role="toolbar" aria-label="格式化工具栏">
      {ITEMS.map((item, i) => {
        if (item.label === '|') {
          return <span key={i} className="toolbar-divider" aria-hidden="true" />
        }
        return (
          <button
            key={i}
            type="button"
            title={item.title}
            aria-label={item.title}
            className="toolbar-btn"
            data-seditor-command={item.command}
          >
            {item.label}
          </button>
        )
      })}
    </div>
  )
}
