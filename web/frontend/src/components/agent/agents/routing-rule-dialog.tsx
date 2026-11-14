import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import type { AgentView, DispatchRule } from "@/api/agents"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type MentionChoice = "any" | "yes" | "no"

interface RoutingRuleDialogProps {
  open: boolean
  rule: DispatchRule
  agents: AgentView[]
  isSaving: boolean
  error: string | null
  onOpenChange: (open: boolean) => void
  onSave: (rule: DispatchRule) => void
}

export function RoutingRuleDialog({
  open,
  rule,
  agents,
  isSaving,
  error,
  onOpenChange,
  onSave,
}: RoutingRuleDialogProps) {
  const { t } = useTranslation()
  const [agent, setAgent] = useState(rule.agent)
  const [name, setName] = useState(rule.name ?? "")
  const [channel, setChannel] = useState(rule.when.channel ?? "")
  const [account, setAccount] = useState(rule.when.account ?? "")
  const [chat, setChat] = useState(rule.when.chat ?? "")
  const [sender, setSender] = useState(rule.when.sender ?? "")
  const [mentioned, setMentioned] = useState<MentionChoice>(
    rule.when.mentioned === true ? "yes" : rule.when.mentioned === false ? "no" : "any",
  )
  const [localError, setLocalError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setAgent(rule.agent)
    setName(rule.name ?? "")
    setChannel(rule.when.channel ?? "")
    setAccount(rule.when.account ?? "")
    setChat(rule.when.chat ?? "")
    setSender(rule.when.sender ?? "")
    setMentioned(
      rule.when.mentioned === true ? "yes" : rule.when.mentioned === false ? "no" : "any",
    )
    setLocalError(null)
  }, [open, rule])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("agents.ruleTitle")}</DialogTitle>
          <DialogDescription>{t("agents.ruleDescription")}</DialogDescription>
        </DialogHeader>

        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            if (
              !channel.trim() &&
              !account.trim() &&
              !chat.trim() &&
              !sender.trim() &&
              mentioned === "any"
            ) {
              setLocalError(t("agents.ruleNeedsCondition"))
              return
            }
            setLocalError(null)
            onSave({
              name: name.trim() || undefined,
              agent,
              when: {
                channel: channel.trim() || undefined,
                account: account.trim() || undefined,
                chat: chat.trim() || undefined,
                sender: sender.trim() || undefined,
                mentioned: mentioned === "any" ? undefined : mentioned === "yes",
              },
            })
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="rule-agent">{t("agents.ruleAgent")}</Label>
            <select
              id="rule-agent"
              value={agent}
              onChange={(event) => setAgent(event.target.value)}
              disabled={isSaving}
              required
              className="border-input bg-background h-9 rounded-md border px-3 text-sm shadow-xs"
            >
              {agents.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.id}
                </option>
              ))}
            </select>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="rule-name">{t("agents.ruleName")}</Label>
            <Input
              id="rule-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t("agents.ruleNamePlaceholder")}
              maxLength={120}
              disabled={isSaving}
            />
          </div>

          <fieldset className="grid gap-3">
            <legend className="text-sm font-semibold">{t("agents.ruleWhen")}</legend>
            <div className="grid gap-2 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="rule-channel">{t("agents.ruleChannel")}</Label>
                <Input
                  id="rule-channel"
                  value={channel}
                  onChange={(event) => setChannel(event.target.value)}
                  placeholder="telegram"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={isSaving}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="rule-account">{t("agents.ruleAccount")}</Label>
                <Input
                  id="rule-account"
                  value={account}
                  onChange={(event) => setAccount(event.target.value)}
                  placeholder={t("agents.ruleAny")}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={isSaving}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="rule-chat">{t("agents.ruleChat")}</Label>
                <Input
                  id="rule-chat"
                  value={chat}
                  onChange={(event) => setChat(event.target.value)}
                  placeholder="group:123"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={isSaving}
                  aria-describedby="rule-chat-help"
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="rule-sender">{t("agents.ruleSender")}</Label>
                <Input
                  id="rule-sender"
                  value={sender}
                  onChange={(event) => setSender(event.target.value)}
                  placeholder={t("agents.ruleAny")}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={isSaving}
                />
              </div>
            </div>
            <p id="rule-chat-help" className="text-muted-foreground text-xs">
              {t("agents.ruleChatHelp")}
            </p>
            <div className="grid gap-1.5">
              <Label htmlFor="rule-mentioned">{t("agents.ruleMentioned")}</Label>
              <select
                id="rule-mentioned"
                value={mentioned}
                onChange={(event) => setMentioned(event.target.value as MentionChoice)}
                disabled={isSaving}
                className="border-input bg-background h-9 rounded-md border px-3 text-sm shadow-xs"
              >
                <option value="any">{t("agents.ruleAny")}</option>
                <option value="yes">{t("agents.ruleMentionedYes")}</option>
                <option value="no">{t("agents.ruleMentionedNo")}</option>
              </select>
            </div>
          </fieldset>

          {(localError || error) && (
            <div
              role="alert"
              className="border-destructive/30 bg-destructive/5 text-destructive rounded-xl border p-3 text-sm"
            >
              {localError || error}
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSaving}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving ? t("common.saving") : t("agents.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
