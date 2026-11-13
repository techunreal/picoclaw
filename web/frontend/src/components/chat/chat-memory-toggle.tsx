import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Switch } from "@/components/ui/switch"
import { setActiveSessionMemoryEnabled } from "@/features/chat/controller"
import { isSessionMemoryEnabled } from "@/features/chat/memory-preference"

interface ChatMemoryToggleProps {
  sessionId: string
}

export function ChatMemoryToggle({ sessionId }: ChatMemoryToggleProps) {
  const { t } = useTranslation()
  const [enabled, setEnabled] = useState(() =>
    isSessionMemoryEnabled(sessionId),
  )

  useEffect(() => {
    setEnabled(isSessionMemoryEnabled(sessionId))
  }, [sessionId])

  const handleChange = (checked: boolean) => {
    setEnabled(checked)
    setActiveSessionMemoryEnabled(checked)
  }

  return (
    <label className="text-muted-foreground flex cursor-pointer items-center gap-2 text-xs">
      <Switch
        size="sm"
        checked={enabled}
        onCheckedChange={handleChange}
        aria-label={t("chat.memoryLabel")}
      />
      <span className="hidden sm:inline">
        {enabled ? t("chat.memoryOn") : t("chat.memoryOff")}
      </span>
    </label>
  )
}
