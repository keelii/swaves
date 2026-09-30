/**
 * markdown.ts
 *
 * Bidirectional conversion between TipTap document JSON and CommonMark
 * Markdown using remark-parse / remark-stringify. remark is based on
 * micromark (100% CommonMark) — the same spec that yuin/goldmark follows on
 * the Go side, so the round-trip is compatible.
 *
 * fromMarkdown(md)  → TipTap doc JSON  (load into editor)
 * toMarkdown(doc)   → Markdown string  (save / sync to textarea)
 */

import { unified } from 'unified'
import remarkParse from 'remark-parse'
import remarkStringify from 'remark-stringify'
import type {
  Root,
  Content,
  PhrasingContent,
  BlockContent,
  DefinitionContent,
  ListItem,
} from 'mdast'

// ── TipTap document shape ────────────────────────────────────────────────────

export interface TipTapMark {
  type: string
  attrs?: Record<string, unknown>
}

export interface TipTapNode {
  type: string
  attrs?: Record<string, unknown>
  content?: TipTapNode[]
  marks?: TipTapMark[]
  text?: string
}

export interface TipTapDoc {
  type: 'doc'
  content: TipTapNode[]
}

// ── Internal fragment helper ─────────────────────────────────────────────────

const FRAGMENT = '__fragment__'

function fragment(nodes: TipTapNode[]): TipTapNode {
  return { type: FRAGMENT, content: nodes }
}

function flattenFragment(node: TipTapNode): TipTapNode[] {
  if (node.type === FRAGMENT) return node.content ?? []
  return [node]
}

// ── mdast → TipTap ───────────────────────────────────────────────────────────

function applyMark(node: TipTapNode, mark: TipTapMark): TipTapNode {
  if (node.type !== 'text') return node
  return { ...node, marks: [...(node.marks ?? []), mark] }
}

function phrasingToTipTap(node: PhrasingContent): TipTapNode | null {
  switch (node.type) {
    case 'text':
      return { type: 'text', text: node.value }

    case 'strong': {
      const mark: TipTapMark = { type: 'bold' }
      const nodes = node.children.flatMap(c => {
        const n = phrasingToTipTap(c)
        return n ? flattenFragment(n).map(x => applyMark(x, mark)) : []
      })
      return nodes.length === 1 ? nodes[0] : fragment(nodes)
    }

    case 'emphasis': {
      const mark: TipTapMark = { type: 'italic' }
      const nodes = node.children.flatMap(c => {
        const n = phrasingToTipTap(c)
        return n ? flattenFragment(n).map(x => applyMark(x, mark)) : []
      })
      return nodes.length === 1 ? nodes[0] : fragment(nodes)
    }

    case 'delete': {
      const mark: TipTapMark = { type: 'strike' }
      const nodes = node.children.flatMap(c => {
        const n = phrasingToTipTap(c)
        return n ? flattenFragment(n).map(x => applyMark(x, mark)) : []
      })
      return nodes.length === 1 ? nodes[0] : fragment(nodes)
    }

    case 'inlineCode':
      return { type: 'text', text: node.value, marks: [{ type: 'code' }] }

    case 'break':
      return { type: 'hardBreak' }

    case 'link': {
      const mark: TipTapMark = {
        type: 'link',
        attrs: { href: node.url, title: node.title ?? null, target: '_blank' },
      }
      const nodes = node.children.flatMap(c => {
        const n = phrasingToTipTap(c)
        return n ? flattenFragment(n).map(x => applyMark(x, mark)) : []
      })
      return nodes.length === 1 ? nodes[0] : fragment(nodes)
    }

    case 'image':
      return {
        type: 'image',
        attrs: { src: node.url, alt: node.alt ?? null, title: node.title ?? null },
      }

    default:
      return null
  }
}

function phrasingListToTipTap(children: PhrasingContent[]): TipTapNode[] {
  return children.flatMap(c => {
    const n = phrasingToTipTap(c)
    return n ? flattenFragment(n) : []
  })
}

type AnyContent = Content | BlockContent | DefinitionContent

function blockToTipTap(node: AnyContent): TipTapNode | null {
  switch (node.type) {
    case 'paragraph':
      return {
        type: 'paragraph',
        content: phrasingListToTipTap(node.children),
      }

    case 'heading':
      return {
        type: 'heading',
        attrs: { level: node.depth },
        content: phrasingListToTipTap(node.children),
      }

    case 'code':
      return {
        type: 'codeBlock',
        attrs: { language: node.lang ?? null },
        content: [{ type: 'text', text: node.value }],
      }

    case 'blockquote': {
      const content = (node.children as AnyContent[])
        .map(blockToTipTap)
        .filter((n): n is TipTapNode => n !== null)
      return { type: 'blockquote', content }
    }

    case 'list': {
      const listType = node.ordered ? 'orderedList' : 'bulletList'
      const items = node.children.map((li: ListItem) => {
        const liContent = (li.children as AnyContent[])
          .map(blockToTipTap)
          .filter((n): n is TipTapNode => n !== null)
        return { type: 'listItem', content: liContent }
      })
      return { type: listType, content: items }
    }

    case 'thematicBreak':
      return { type: 'horizontalRule' }

    default:
      return null
  }
}

export function fromMarkdown(markdown: string): TipTapDoc {
  const tree = unified().use(remarkParse).parse(markdown) as Root
  const content = (tree.children as AnyContent[])
    .map(blockToTipTap)
    .filter((n): n is TipTapNode => n !== null)
  if (content.length === 0) content.push({ type: 'paragraph' })
  return { type: 'doc', content }
}

// ── TipTap → mdast ───────────────────────────────────────────────────────────

function tiptapInlineToMdast(node: TipTapNode): PhrasingContent | null {
  if (node.type === 'hardBreak') return { type: 'break' }

  if (node.type === 'image') {
    return {
      type: 'image',
      url: String(node.attrs?.src ?? ''),
      alt: node.attrs?.alt ? String(node.attrs.alt) : null,
      title: node.attrs?.title ? String(node.attrs.title) : null,
    }
  }

  if (node.type !== 'text') return null

  const text = node.text ?? ''
  const marks = node.marks ?? []

  // Code mark → inlineCode (applied first, supersedes other marks)
  const codeMark = marks.find(m => m.type === 'code')
  if (codeMark) return { type: 'inlineCode', value: text }

  // Build base text node then wrap in mark nodes
  let result: PhrasingContent = { type: 'text', value: text }

  const linkMark = marks.find(m => m.type === 'link')
  if (linkMark) {
    result = {
      type: 'link',
      url: String(linkMark.attrs?.href ?? ''),
      title: linkMark.attrs?.title ? String(linkMark.attrs.title) : null,
      children: [result],
    }
  }

  if (marks.find(m => m.type === 'italic')) {
    result = { type: 'emphasis', children: [result] }
  }

  if (marks.find(m => m.type === 'bold')) {
    result = { type: 'strong', children: [result] }
  }

  if (marks.find(m => m.type === 'strike')) {
    result = { type: 'delete', children: [result] }
  }

  return result
}

function tiptapBlockToMdast(node: TipTapNode): BlockContent | null {
  switch (node.type) {
    case 'paragraph': {
      const children = (node.content ?? [])
        .map(tiptapInlineToMdast)
        .filter((n): n is PhrasingContent => n !== null)
      return { type: 'paragraph', children }
    }

    case 'heading': {
      const depth = (node.attrs?.level as number) ?? 1
      const children = (node.content ?? [])
        .map(tiptapInlineToMdast)
        .filter((n): n is PhrasingContent => n !== null)
      return {
        type: 'heading',
        depth: Math.min(Math.max(depth, 1), 6) as 1 | 2 | 3 | 4 | 5 | 6,
        children,
      }
    }

    case 'codeBlock': {
      const lang = node.attrs?.language ? String(node.attrs.language) : null
      const value = (node.content ?? []).map(n => n.text ?? '').join('')
      return { type: 'code', value, lang, meta: null }
    }

    case 'blockquote': {
      const children = (node.content ?? [])
        .map(tiptapBlockToMdast)
        .filter((n): n is BlockContent => n !== null)
      return { type: 'blockquote', children }
    }

    case 'bulletList': {
      const children = (node.content ?? []).map(li => {
        const liContent = (li.content ?? [])
          .map(tiptapBlockToMdast)
          .filter((n): n is BlockContent => n !== null)
        const item: ListItem = { type: 'listItem', spread: false, children: liContent }
        return item
      })
      return { type: 'list', ordered: false, spread: false, children }
    }

    case 'orderedList': {
      const start = (node.attrs?.start as number) ?? 1
      const children = (node.content ?? []).map(li => {
        const liContent = (li.content ?? [])
          .map(tiptapBlockToMdast)
          .filter((n): n is BlockContent => n !== null)
        const item: ListItem = { type: 'listItem', spread: false, children: liContent }
        return item
      })
      return { type: 'list', ordered: true, start, spread: false, children }
    }

    case 'horizontalRule':
      return { type: 'thematicBreak' }

    default:
      return null
  }
}

export function toMarkdown(doc: TipTapDoc): string {
  const root: Root = {
    type: 'root',
    children: (doc.content ?? [])
      .map(tiptapBlockToMdast)
      .filter((n): n is BlockContent => n !== null),
  }
  return String(
    unified()
      .use(remarkStringify, {
        bullet: '-',
        emphasis: '_',
        strong: '*',
        fence: '`',
        fences: true,
        incrementListMarker: false,
      })
      .stringify(root),
  )
}
