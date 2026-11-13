import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Switch } from "@/components/ui/switch"
import { setActiveSessionMemoryEnabled } from "@/features/chat/controller"
import { isSessionMemoryEnabled } from "@/features/chat/memory-preference"
import { isIncognitoSession } from "@/features/chat/state"

interface ChatMemoryToggleProps {
  sessionId: string
}

export function ChatMemoryToggle({ sessionId }: ChatMemoryToggleProps) {
  const { t } = useTranslation()
  const incognito = isIncognitoSession(sessionId)
  const [enabled, setEnabled] = useState(
    () => !incognito && isSessionMemoryEnabled(sessionId),
  )

  useEffect(() => {
    setEnabled(
      !isIncognitoSession(sessionId) && isSessionMemoryEnabled(sessionId),
    )
  }, [sessionId])

  const handleChange = (checked: boolean) => {
    if (incognito) {
      return
    }
    setEnabled(checked)
    setActiveSessionMemoryEnabled(checked)
  }

  return (
    <label className="text-muted-foreground flex cursor-pointer items-center gap-2 text-xs">
      <Switch
        size="sm"
        checked={enabled}
        disabled={incognito}
        onCheckedChange={handleChange}
        aria-label={t("chat.memoryLabel")}
      />
      <span className="hidden sm:inline">
        {enabled ? t("chat.memoryOn") : t("chat.memoryOff")}
      </span>
    </label>
  )
}
