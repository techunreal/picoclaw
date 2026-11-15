import {
  IconArrowAutofitDown,
  IconEdit,
  IconKey,
  IconStar,
  IconStarFilled,
  IconTrash,
} from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import type { ModelInfo } from "@/api/models"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

interface ModelCardProps {
  model: ModelInfo
  onEdit: (model: ModelInfo) => void
  onSetDefault: (model: ModelInfo) => void
  onToggleFallback: (model: ModelInfo) => void
  onToggleEnabled: (model: ModelInfo) => void
  onDelete: (model: ModelInfo) => void
  inFallbackChain: boolean
  isDefault: boolean
  defaultChainAllowed: boolean
  fallbackDefaultConflict: boolean
  deleteDisabled: boolean
}

export function ModelCard({
  model,
  onEdit,
  onSetDefault,
  onToggleFallback,
  onToggleEnabled,
  onDelete,
  inFallbackChain,
  isDefault,
  defaultChainAllowed,
  fallbackDefaultConflict,
  deleteDisabled,
}: ModelCardProps) {
  const { t } = useTranslation()
  const isOAuth = model.auth_method === "oauth"
  const status = model.status
  const statusLabel = t(`models.status.${status}`)
  const canSetDefault =
    model.enabled && model.available && !isDefault && defaultChainAllowed
  const canUseInFallback =
    model.enabled &&
    model.available &&
    !isDefault &&
    !fallbackDefaultConflict &&
    defaultChainAllowed

  const setDefaultLabel = t("models.action.setDefault")
  const setDefaultDisabledReason = (() => {
    if (!model.enabled)
      return t("models.action.setDefaultDisabled.disabled")
    if (!model.available)
      return t("models.action.setDefaultDisabled.unavailable")
    if (isDefault) return t("models.action.setDefaultDisabled.isDefault")
    if (model.is_virtual) return t("models.action.setDefaultDisabled.isVirtual")
    if (!defaultChainAllowed) {
      return t("models.action.setDefaultDisabled.unsupportedProvider")
    }
    return setDefaultLabel
  })()

  const editLabel = t("models.action.edit")
  const toggleFallbackLabel = inFallbackChain
    ? t("models.action.removeFromFallback")
    : t("models.action.addToFallback")
  const toggleFallbackDisabledReason = (() => {
    if (!model.enabled)
      return t("models.action.addToFallbackDisabled.disabled")
    if (!model.available)
      return t("models.action.setDefaultDisabled.unavailable")
    if (model.is_virtual) return t("models.action.setDefaultDisabled.isVirtual")
    if (!defaultChainAllowed) {
      return t("models.action.setDefaultDisabled.unsupportedProvider")
    }
    if (isDefault || fallbackDefaultConflict)
      return t("models.action.addToFallbackDisabled.isDefault")
    return toggleFallbackLabel
  })()
  const deleteLabel = t("models.action.delete")
  const deleteDisabledReason = deleteDisabled
    ? t("models.action.deleteDisabled.isDefault")
    : deleteLabel
  const keyCount = model.api_key_count ?? model.api_keys?.length ?? 0

  return (
    <div
      className={[
        "group/card hover:bg-muted/30 flex w-full flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg border px-3 py-2 transition-colors hover:shadow-xs",
        !model.enabled
          ? "border-border/40 bg-card/40 opacity-70"
          : model.available
            ? "border-border/60 bg-card"
            : "border-border/50 bg-card/60",
      ].join(" ")}
    >
      <span
        className={[
          "h-2 w-2 shrink-0 rounded-full",
          isDefault
            ? "bg-green-400 shadow-[0_0_0_2px_rgba(74,222,128,0.35)]"
            : status === "available"
              ? "bg-green-500"
              : status === "unreachable"
                ? "bg-amber-500"
                : "bg-muted-foreground/25",
        ].join(" ")}
        title={statusLabel}
      />

      <span className="text-foreground max-w-[16rem] truncate text-sm font-semibold">
        {model.model_name}
      </span>

      {isDefault && (
        <span className="bg-primary/10 text-primary shrink-0 rounded px-1.5 py-0.5 text-[10px] leading-none font-medium">
          {t("models.badge.default")}
        </span>
      )}
      {model.is_virtual && (
        <span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[10px] leading-none font-medium">
          {t("models.badge.virtual")}
        </span>
      )}
      {!model.enabled && (
        <span className="bg-destructive/10 text-destructive shrink-0 rounded px-1.5 py-0.5 text-[10px] leading-none font-medium">
          {t("models.badge.disabled")}
        </span>
      )}

      <span className="text-muted-foreground min-w-0 flex-1 truncate font-mono text-xs">
        {model.model}
      </span>

      <span className="text-muted-foreground/60 hidden max-w-[12rem] shrink-0 truncate text-[11px] lg:inline">
        {isOAuth ? (
          "OAuth"
        ) : status === "available" && model.api_key ? (
          <span className="inline-flex items-center gap-1 font-mono">
            <IconKey className="size-3" />
            {model.api_key}
            {keyCount > 1 && ` (${keyCount})`}
          </span>
        ) : status === "available" ? (
          t("models.keylessHint")
        ) : (
          statusLabel
        )}
      </span>

      <div className="ml-auto flex shrink-0 items-center gap-0.5">
        {isDefault ? (
          <span className="text-primary p-1" title={t("models.badge.default")}>
            <IconStarFilled className="size-3.5" />
          </span>
        ) : (
          <Tooltip delayDuration={!canSetDefault ? 0 : 700}>
            <TooltipTrigger asChild>
              <span
                className={!canSetDefault ? "cursor-not-allowed" : undefined}
                tabIndex={!canSetDefault ? 0 : undefined}
                role={!canSetDefault ? "button" : undefined}
                aria-disabled={!canSetDefault ? true : undefined}
                aria-label={!canSetDefault ? setDefaultLabel : undefined}
                title={!canSetDefault ? setDefaultLabel : undefined}
              >
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onSetDefault(model)}
                  disabled={!canSetDefault}
                  aria-label={setDefaultLabel}
                  title={setDefaultLabel}
                >
                  <IconStar className="size-3.5" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{setDefaultDisabledReason}</TooltipContent>
          </Tooltip>
        )}

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => onEdit(model)}
          aria-label={editLabel}
          title={editLabel}
        >
          <IconEdit className="size-3.5" />
        </Button>

        <Tooltip delayDuration={canUseInFallback ? 700 : 0}>
          <TooltipTrigger asChild>
            <span
              className={!canUseInFallback ? "cursor-not-allowed" : undefined}
              tabIndex={!canUseInFallback ? 0 : undefined}
              role={!canUseInFallback ? "button" : undefined}
              aria-disabled={!canUseInFallback ? true : undefined}
              aria-label={!canUseInFallback ? toggleFallbackLabel : undefined}
              title={!canUseInFallback ? toggleFallbackLabel : undefined}
            >
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onToggleFallback(model)}
                disabled={!canUseInFallback}
                aria-label={toggleFallbackLabel}
                title={toggleFallbackLabel}
              >
                <IconArrowAutofitDown
                  className={[
                    "size-3.5",
                    inFallbackChain ? "text-primary" : "",
                  ].join(" ")}
                />
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{toggleFallbackDisabledReason}</TooltipContent>
        </Tooltip>

        <Tooltip delayDuration={deleteDisabled ? 0 : 700}>
          <TooltipTrigger asChild>
            <span
              className={deleteDisabled ? "cursor-not-allowed" : undefined}
              tabIndex={deleteDisabled ? 0 : undefined}
              role={deleteDisabled ? "button" : undefined}
              aria-disabled={deleteDisabled ? true : undefined}
              aria-label={deleteDisabled ? deleteLabel : undefined}
              title={deleteDisabled ? deleteLabel : undefined}
            >
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onDelete(model)}
                disabled={deleteDisabled}
                aria-label={deleteLabel}
                title={deleteLabel}
                className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
              >
                <IconTrash className="size-3.5" />
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{deleteDisabledReason}</TooltipContent>
        </Tooltip>
      </div>

      <span className="text-muted-foreground ml-1 shrink-0 text-[11px]">
        {t("models.enableLabel")}
      </span>
      <Switch
        checked={model.enabled}
        onCheckedChange={() => onToggleEnabled(model)}
        disabled={model.is_virtual}
        aria-label={t("models.action.toggleEnabled")}
      />
    </div>
  )
}
