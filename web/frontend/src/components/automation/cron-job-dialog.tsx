import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import type { CronJob, CronJobInput } from "@/api/cron"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

type SchedulePreset = "daily" | "interval" | "once" | "cron"

interface CronJobDialogProps {
  open: boolean
  job: CronJob | null
  isSaving: boolean
  onOpenChange: (open: boolean) => void
  onSave: (input: CronJobInput) => void
}

interface FormState {
  name: string
  message: string
  preset: SchedulePreset
  dailyTime: string
  intervalAmount: string
  intervalUnit: "minutes" | "hours"
  onceAt: string
  cronExpr: string
  channel: string
  to: string
}

const SIMPLE_DAILY_CRON = /^(\d{1,2})\s+(\d{1,2})\s+\*\s+\*\s+\*$/

function pad(value: number): string {
  return String(value).padStart(2, "0")
}

function toLocalDateTimeInput(ms: number): string {
  const date = new Date(ms)
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}`
}

function buildInitialState(job: CronJob | null): FormState {
  const state: FormState = {
    name: job?.name ?? "",
    message: job?.payload.message ?? "",
    preset: "daily",
    dailyTime: "09:00",
    intervalAmount: "1",
    intervalUnit: "hours",
    onceAt: toLocalDateTimeInput(Date.now() + 60 * 60 * 1000),
    cronExpr: "0 9 * * *",
    channel: job?.payload.channel ?? "",
    to: job?.payload.to ?? "",
  }
  if (!job) {
    return state
  }

  switch (job.schedule.kind) {
    case "at":
      state.preset = "once"
      if (job.schedule.atMs) {
        state.onceAt = toLocalDateTimeInput(job.schedule.atMs)
      }
      break
    case "every": {
      state.preset = "interval"
      const everyMs = job.schedule.everyMs ?? 3_600_000
      if (everyMs >= 3_600_000 && everyMs % 3_600_000 === 0) {
        state.intervalUnit = "hours"
        state.intervalAmount = String(everyMs / 3_600_000)
      } else {
        state.intervalUnit = "minutes"
        state.intervalAmount = String(Math.max(1, Math.round(everyMs / 60_000)))
      }
      break
    }
    case "cron": {
      const expr = job.schedule.expr?.trim() ?? ""
      const daily = expr.match(SIMPLE_DAILY_CRON)
      if (daily) {
        state.preset = "daily"
        state.dailyTime = `${daily[2].padStart(2, "0")}:${daily[1].padStart(2, "0")}`
      } else {
        state.preset = "cron"
        state.cronExpr = expr
      }
      break
    }
  }
  return state
}

function buildSchedule(form: FormState): CronJobInput["schedule"] | null {
  if (form.preset === "daily") {
    const match = form.dailyTime.match(/^(\d{2}):(\d{2})$/)
    if (!match) {
      return null
    }
    const hour = Number(match[1])
    const minute = Number(match[2])
    if (hour > 23 || minute > 59) {
      return null
    }
    return { kind: "cron", expr: `${minute} ${hour} * * *` }
  }

  if (form.preset === "interval") {
    const amount = Number(form.intervalAmount)
    if (!Number.isFinite(amount) || amount <= 0) {
      return null
    }
    const everyMs =
      form.intervalUnit === "hours" ? amount * 3_600_000 : amount * 60_000
    if (everyMs < 60_000) {
      return null
    }
    return { kind: "every", every_ms: Math.round(everyMs) }
  }

  if (form.preset === "once") {
    const at = new Date(form.onceAt)
    if (Number.isNaN(at.getTime())) {
      return null
    }
    return { kind: "at", at_ms: at.getTime() }
  }

  const expr = form.cronExpr.trim()
  if (!expr) {
    return null
  }
  return { kind: "cron", expr }
}

export function CronJobDialog({
  open,
  job,
  isSaving,
  onOpenChange,
  onSave,
}: CronJobDialogProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState<FormState>(() => buildInitialState(job))
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setForm(buildInitialState(job))
      setError(null)
    }
  }, [open, job])

  const scheduleHint = useMemo(() => {
    const schedule = buildSchedule(form)
    if (!schedule) {
      return null
    }
    if (schedule.kind === "cron" && schedule.expr) {
      return t("cron.scheduleCronHint", { expr: schedule.expr })
    }
    return null
  }, [form, t])

  const setField = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const handleSubmit = () => {
    const message = form.message.trim()
    if (!message) {
      setError(t("cron.messageRequired"))
      return
    }

    const schedule = buildSchedule(form)
    if (!schedule) {
      setError(t("cron.scheduleInvalid"))
      return
    }
    if (schedule.kind === "at" && (schedule.at_ms ?? 0) <= Date.now()) {
      setError(t("cron.scheduleMustBeFuture"))
      return
    }

    setError(null)
    onSave({
      name: form.name.trim(),
      message,
      schedule,
      channel: form.channel.trim() || undefined,
      to: form.to.trim() || undefined,
      enabled: job?.enabled ?? true,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {job ? t("cron.editTitle") : t("cron.createTitle")}
          </DialogTitle>
          <DialogDescription>{t("cron.dialogDescription")}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="cron-name">{t("cron.field.name")}</Label>
            <Input
              id="cron-name"
              value={form.name}
              onChange={(e) => setField("name", e.target.value)}
              placeholder={t("cron.field.namePlaceholder")}
              maxLength={120}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="cron-message">{t("cron.field.message")}</Label>
            <Textarea
              id="cron-message"
              value={form.message}
              onChange={(e) => setField("message", e.target.value)}
              placeholder={t("cron.field.messagePlaceholder")}
              rows={4}
              maxLength={8000}
            />
            <p className="text-muted-foreground text-xs">
              {t("cron.field.messageHint")}
            </p>
          </div>

          <div className="grid gap-2">
            <Label>{t("cron.field.schedule")}</Label>
            <Select
              value={form.preset}
              onValueChange={(value) =>
                setField("preset", value as SchedulePreset)
              }
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="daily">{t("cron.preset.daily")}</SelectItem>
                <SelectItem value="interval">
                  {t("cron.preset.interval")}
                </SelectItem>
                <SelectItem value="once">{t("cron.preset.once")}</SelectItem>
                <SelectItem value="cron">{t("cron.preset.cron")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {form.preset === "daily" && (
            <div className="grid gap-2">
              <Label htmlFor="cron-time">{t("cron.field.time")}</Label>
              <Input
                id="cron-time"
                type="time"
                value={form.dailyTime}
                onChange={(e) => setField("dailyTime", e.target.value)}
              />
            </div>
          )}

          {form.preset === "interval" && (
            <div className="grid grid-cols-[1fr_140px] gap-2">
              <div className="grid gap-2">
                <Label htmlFor="cron-interval">
                  {t("cron.field.interval")}
                </Label>
                <Input
                  id="cron-interval"
                  type="number"
                  min={1}
                  value={form.intervalAmount}
                  onChange={(e) =>
                    setField("intervalAmount", e.target.value)
                  }
                />
              </div>
              <div className="grid gap-2">
                <Label>{t("cron.field.unit")}</Label>
                <Select
                  value={form.intervalUnit}
                  onValueChange={(value) =>
                    setField("intervalUnit", value as "minutes" | "hours")
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="minutes">
                      {t("cron.unit.minutes")}
                    </SelectItem>
                    <SelectItem value="hours">
                      {t("cron.unit.hours")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          {form.preset === "once" && (
            <div className="grid gap-2">
              <Label htmlFor="cron-once">{t("cron.field.onceAt")}</Label>
              <Input
                id="cron-once"
                type="datetime-local"
                value={form.onceAt}
                onChange={(e) => setField("onceAt", e.target.value)}
              />
            </div>
          )}

          {form.preset === "cron" && (
            <div className="grid gap-2">
              <Label htmlFor="cron-expr">{t("cron.field.cronExpr")}</Label>
              <Input
                id="cron-expr"
                value={form.cronExpr}
                onChange={(e) => setField("cronExpr", e.target.value)}
                placeholder="0 9 * * *"
                className="font-mono"
              />
              <p className="text-muted-foreground text-xs">
                {t("cron.field.cronExprHint")}
              </p>
            </div>
          )}

          {scheduleHint && (
            <p className="text-muted-foreground text-xs">{scheduleHint}</p>
          )}

          <details className="border-border/60 rounded-lg border p-3">
            <summary className="cursor-pointer text-sm font-medium">
              {t("cron.field.advanced")}
            </summary>
            <div className="mt-3 grid gap-3">
              <div className="grid gap-2">
                <Label htmlFor="cron-channel">{t("cron.field.channel")}</Label>
                <Input
                  id="cron-channel"
                  value={form.channel}
                  onChange={(e) => setField("channel", e.target.value)}
                  placeholder="cli"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="cron-target">{t("cron.field.target")}</Label>
                <Input
                  id="cron-target"
                  value={form.to}
                  onChange={(e) => setField("to", e.target.value)}
                  placeholder="direct"
                />
                <p className="text-muted-foreground text-xs">
                  {t("cron.field.targetHint")}
                </p>
              </div>
            </div>
          </details>

          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isSaving}
          >
            {t("cron.cancel")}
          </Button>
          <Button type="button" onClick={handleSubmit} disabled={isSaving}>
            {isSaving ? t("cron.saving") : t("cron.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
