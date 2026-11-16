import { useEffect, useState } from "react"

import { agentAvatarUrl, type AgentAvatar as AgentAvatarData } from "@/api/agents"
import { cn } from "@/lib/utils"

type AvatarSize = "sm" | "md" | "lg"

const sizeClasses: Record<AvatarSize, string> = {
  sm: "size-5 text-[13px]",
  md: "size-7 text-base",
  lg: "size-10 text-xl",
}

const FALLBACK_EMOJIS = [
  "🦉",
  "🦊",
  "🐼",
  "🐸",
  "🐙",
  "🦁",
  "🐧",
  "🦄",
  "🐻",
  "🐨",
  "🦋",
  "🐬",
  "🐢",
  "🦖",
  "🐳",
  "🦜",
]

function fallbackEmoji(seed: string): string {
  let hash = 0
  for (let i = 0; i < seed.length; i += 1) {
    hash = (hash * 31 + seed.charCodeAt(i)) >>> 0
  }
  return FALLBACK_EMOJIS[hash % FALLBACK_EMOJIS.length]
}

interface AgentAvatarProps {
  agentId?: string
  avatar?: AgentAvatarData | null
  name?: string
  fallback?: string
  size?: AvatarSize
  className?: string
}

export function AgentAvatar({
  agentId,
  avatar,
  name,
  fallback,
  size = "sm",
  className,
}: AgentAvatarProps) {
  const [failedVersion, setFailedVersion] = useState<number | undefined>(undefined)

  useEffect(() => {
    setFailedVersion(undefined)
  }, [avatar?.image, avatar?.version])

  const showImage =
    Boolean(agentId && avatar?.image) && failedVersion !== avatar?.version
  const text =
    avatar?.emoji?.trim() ||
    fallback ||
    fallbackEmoji(agentId || name || "agent")

  return (
    <span
      className={cn(
        "bg-muted text-muted-foreground inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full leading-none",
        sizeClasses[size],
        className,
      )}
      role="img"
      aria-label={name?.trim() || agentId || "agent"}
    >
      {showImage ? (
        <img
          src={agentAvatarUrl(agentId as string, avatar?.version)}
          alt={name?.trim() || agentId}
          className="size-full object-cover"
          onError={() => setFailedVersion(avatar?.version)}
        />
      ) : (
        text
      )}
    </span>
  )
}
