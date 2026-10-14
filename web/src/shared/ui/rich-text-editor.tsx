import { EditorContent, useEditor, useEditorState, type Editor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import {
  BoldIcon,
  CodeIcon,
  Heading2Icon,
  Heading3Icon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  QuoteIcon,
  Redo2Icon,
  StrikethroughIcon,
  UnderlineIcon,
  Undo2Icon,
  UnlinkIcon,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/shared/lib/utils'

function ToolButton({
  editor,
  icon: Icon,
  label,
  active,
  disabled,
  onClick,
}: {
  editor: Editor
  icon: LucideIcon
  label: string
  active?: boolean
  disabled?: boolean
  onClick: (editor: Editor) => void
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={active}
      disabled={disabled}
      // Düğmeye basmak editörün odağını ve seçimini almasın.
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => onClick(editor)}
      className={cn(
        'inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40',
        active && 'bg-primary/10 text-primary',
      )}
    >
      <Icon className="size-4" />
    </button>
  )
}

/** toolState, araç çubuğu düğmelerinin etkin ve kullanılabilir olma durumudur. */
function toolState(e: Editor) {
  return {
    bold: e.isActive('bold'),
    italic: e.isActive('italic'),
    underline: e.isActive('underline'),
    strike: e.isActive('strike'),
    h2: e.isActive('heading', { level: 2 }),
    h3: e.isActive('heading', { level: 3 }),
    bullet: e.isActive('bulletList'),
    ordered: e.isActive('orderedList'),
    quote: e.isActive('blockquote'),
    code: e.isActive('codeBlock'),
    link: e.isActive('link'),
    undo: e.can().undo(),
    redo: e.can().redo(),
  }
}

/**
 * RichTextEditor, araç çubuklu zengin metin editörüdür (Tiptap): başlıklar, kalın, italik,
 * altı çizili, üstü çizili, listeler, alıntı, kod bloğu ve bağlantı. Değer HTML'dir; sunucu
 * kaydederken izin listesiyle temizler.
 */
export function RichTextEditor({
  value,
  onChange,
  label,
  placeholder,
  minHeight = 'min-h-40',
}: {
  value: string
  onChange: (html: string) => void
  label: string
  placeholder?: string
  minHeight?: string
}) {
  const { t } = useTranslation()
  const editor = useEditor({
    extensions: [
      StarterKit.configure({ link: { openOnClick: false, autolink: true, defaultProtocol: 'https' } }),
    ],
    content: value,
    editorProps: {
      attributes: {
        'aria-label': label,
        'aria-multiline': 'true',
        role: 'textbox',
        class: cn('rich-text px-3 py-2.5 outline-none', minHeight),
        ...(placeholder ? { 'data-placeholder': placeholder } : {}),
      },
    },
    onUpdate: ({ editor }) => onChange(editor.isEmpty ? '' : editor.getHTML()),
    immediatelyRender: false,
  })
  // useEditorState, editör ilk oluştuğu çizimde henüz null döner (ilk işlemde güncellenir):
  // o çizimde durum doğrudan editörden okunur.
  const watched = useEditorState({ editor, selector: ({ editor: e }) => (e ? toolState(e) : null) })
  if (!editor) {
    return <div className={cn('rounded-lg border bg-card', minHeight)} />
  }
  const state = watched ?? toolState(editor)
  const link = (e: Editor) => {
    const url = window.prompt(
      t('richText.linkPrompt'),
      (e.getAttributes('link').href as string | undefined) ?? 'https://',
    )
    if (url === null) {
      return
    }
    if (url === '') {
      e.chain().focus().unsetLink().run()
      return
    }
    e.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
  }
  return (
    <div className="overflow-hidden rounded-lg border border-input bg-card focus-within:border-primary/60 focus-within:ring-4 focus-within:ring-ring/20">
      <div
        role="toolbar"
        aria-label={t('richText.toolbar')}
        className="flex flex-wrap gap-0.5 border-b bg-muted/40 p-1"
      >
        <ToolButton
          editor={editor}
          icon={Heading2Icon}
          label={t('richText.h2')}
          active={state.h2}
          onClick={(e) => e.chain().focus().toggleHeading({ level: 2 }).run()}
        />
        <ToolButton
          editor={editor}
          icon={Heading3Icon}
          label={t('richText.h3')}
          active={state.h3}
          onClick={(e) => e.chain().focus().toggleHeading({ level: 3 }).run()}
        />
        <ToolButton
          editor={editor}
          icon={BoldIcon}
          label={t('richText.bold')}
          active={state.bold}
          onClick={(e) => e.chain().focus().toggleBold().run()}
        />
        <ToolButton
          editor={editor}
          icon={ItalicIcon}
          label={t('richText.italic')}
          active={state.italic}
          onClick={(e) => e.chain().focus().toggleItalic().run()}
        />
        <ToolButton
          editor={editor}
          icon={UnderlineIcon}
          label={t('richText.underline')}
          active={state.underline}
          onClick={(e) => e.chain().focus().toggleUnderline().run()}
        />
        <ToolButton
          editor={editor}
          icon={StrikethroughIcon}
          label={t('richText.strike')}
          active={state.strike}
          onClick={(e) => e.chain().focus().toggleStrike().run()}
        />
        <span aria-hidden className="mx-1 w-px self-stretch bg-border" />
        <ToolButton
          editor={editor}
          icon={ListIcon}
          label={t('richText.bullet')}
          active={state.bullet}
          onClick={(e) => e.chain().focus().toggleBulletList().run()}
        />
        <ToolButton
          editor={editor}
          icon={ListOrderedIcon}
          label={t('richText.ordered')}
          active={state.ordered}
          onClick={(e) => e.chain().focus().toggleOrderedList().run()}
        />
        <ToolButton
          editor={editor}
          icon={QuoteIcon}
          label={t('richText.quote')}
          active={state.quote}
          onClick={(e) => e.chain().focus().toggleBlockquote().run()}
        />
        <ToolButton
          editor={editor}
          icon={CodeIcon}
          label={t('richText.code')}
          active={state.code}
          onClick={(e) => e.chain().focus().toggleCodeBlock().run()}
        />
        <span aria-hidden className="mx-1 w-px self-stretch bg-border" />
        <ToolButton
          editor={editor}
          icon={LinkIcon}
          label={t('richText.link')}
          active={state.link}
          onClick={link}
        />
        <ToolButton
          editor={editor}
          icon={UnlinkIcon}
          label={t('richText.unlink')}
          disabled={!state.link}
          onClick={(e) => e.chain().focus().unsetLink().run()}
        />
        <span aria-hidden className="mx-1 w-px self-stretch bg-border" />
        <ToolButton
          editor={editor}
          icon={Undo2Icon}
          label={t('richText.undo')}
          disabled={!state.undo}
          onClick={(e) => e.chain().focus().undo().run()}
        />
        <ToolButton
          editor={editor}
          icon={Redo2Icon}
          label={t('richText.redo')}
          disabled={!state.redo}
          onClick={(e) => e.chain().focus().redo().run()}
        />
      </div>
      <EditorContent editor={editor} />
    </div>
  )
}
