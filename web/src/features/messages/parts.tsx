import { MonitorPlayIcon, UsersRoundIcon } from 'lucide-react'

import type { Conversation } from '@/shared/api/generated'
import { Avatar } from '@/shared/ui/avatar'
import { IconChip } from '@/shared/ui/icon-chip'

import { splitName } from './names'

/** ConversationIcon, konuşmanın simgesidir: birebirde kişinin baş harfleri, grupta ve şube grubunda ikon. */
export function ConversationIcon({ c, className }: { c: Conversation; className?: string }) {
  if (c.kind === 'DIRECT' && c.peer) {
    const [first, last] = splitName(c.peer.name)
    return <Avatar firstName={first} lastName={last} className={className ?? 'size-10'} />
  }
  return c.kind === 'SECTION' ? (
    <IconChip icon={MonitorPlayIcon} tone="indigo" className={className} />
  ) : (
    <IconChip icon={UsersRoundIcon} tone="violet" className={className} />
  )
}
