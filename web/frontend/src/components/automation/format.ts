import dayjs from "dayjs"
import type { TFunction } from "i18next"

import type { CronSchedule } from "@/api/cron"

const SIMPLE_DAILY_CRON = /^(\d{1,2})\s+(\d{1,2})\s+\*\s+\*\s+\*$/

export function describeSchedule(
  schedule: CronSchedule,
  t: TFunction,
): string {
  switch (schedule.kind) {
    case "at":
      return schedule.atMs
        ? t("cron.scheduleOnce", {
            time: dayjs(schedule.atMs).format("YYYY-MM-DD HH:mm"),
          })
        : t("cron.scheduleUnknown")
    case "every": {
      const everyMs = schedule.everyMs ?? 0
      if (everyMs >= 3_600_000 && everyMs % 3_600_000 === 0) {
        return t("cron.scheduleEveryHours", { count: everyMs / 3_600_000 })
      }
      if (everyMs >= 60_000) {
        return t("cron.scheduleEveryMinutes", {
          count: Math.round(everyMs / 60_000),
        })
      }
      return t("cron.scheduleUnknown")
    }
    case "cron": {
      const expr = schedule.expr?.trim() ?? ""
      const daily = expr.match(SIMPLE_DAILY_CRON)
      if (daily) {
        const hour = daily[2].padStart(2, "0")
        const minute = daily[1].padStart(2, "0")
        return t("cron.scheduleDaily", { time: `${hour}:${minute}` })
      }
      return t("cron.scheduleCron", { expr: expr })
    }
    default:
      return t("cron.scheduleUnknown")
  }
}

export function formatRunTime(
  value: number | undefined,
  t: TFunction,
): string {
  if (!value) {
    return t("cron.never")
  }
  return dayjs(value).format("YYYY-MM-DD HH:mm")
}
