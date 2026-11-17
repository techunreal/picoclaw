import { useTranslation } from "react-i18next"

import type { ModelInfo } from "@/api/models"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

interface ModelSelectorProps {
  defaultModelName: string
  apiKeyModels: ModelInfo[]
  oauthModels: ModelInfo[]
  localModels: ModelInfo[]
  badgeLabel?: string
  badgeTitle?: string
  globalDefaultModelName?: string
  canUseGlobalDefault?: boolean
  disabled?: boolean
  onValueChange: (modelName: string) => void
}

export function ModelSelector({
  defaultModelName,
  apiKeyModels,
  oauthModels,
  localModels,
  badgeLabel = "",
  badgeTitle,
  globalDefaultModelName = "",
  canUseGlobalDefault = false,
  disabled = false,
  onValueChange,
}: ModelSelectorProps) {
  const { t } = useTranslation()
  const rawDefaultModel =
    defaultModelName &&
    ![...apiKeyModels, ...oauthModels, ...localModels].some(
      (model) => model.model_name === defaultModelName,
    )
      ? defaultModelName
      : ""

  return (
    <Select
      value={defaultModelName}
      onValueChange={onValueChange}
      disabled={disabled}
    >
      <SelectTrigger
        size="sm"
        title={badgeTitle}
        className="text-muted-foreground hover:text-foreground focus-visible:border-input h-8 max-w-[160px] min-w-[80px] gap-1.5 bg-transparent shadow-none focus-visible:ring-0 sm:max-w-[220px]"
      >
        <SelectValue placeholder={t("chat.noModel")} />
        {badgeLabel && (
          <span className="bg-primary/10 text-primary shrink-0 rounded px-1 py-0.5 text-[9px] leading-none font-semibold uppercase">
            {badgeLabel}
          </span>
        )}
      </SelectTrigger>
      <SelectContent position="popper" align="start">
        {canUseGlobalDefault && (
          <>
            <SelectItem value="__global__">
              {t("models.useGlobalDefault", {
                model: globalDefaultModelName || defaultModelName,
              })}
            </SelectItem>
            <SelectSeparator />
          </>
        )}
        {rawDefaultModel && (
          <SelectItem value={rawDefaultModel}>{rawDefaultModel}</SelectItem>
        )}
        {rawDefaultModel &&
          (apiKeyModels.length > 0 ||
            oauthModels.length > 0 ||
            localModels.length > 0) && <SelectSeparator />}
        {apiKeyModels.length > 0 && (
          <SelectGroup>
            <SelectLabel>{t("chat.modelGroup.apikey")}</SelectLabel>
            {apiKeyModels.map((model) => (
              <SelectItem key={model.index} value={model.model_name}>
                {model.model_name}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
        {apiKeyModels.length > 0 &&
          (oauthModels.length > 0 || localModels.length > 0) && (
            <SelectSeparator />
          )}

        {oauthModels.length > 0 && (
          <SelectGroup>
            <SelectLabel>{t("chat.modelGroup.oauth")}</SelectLabel>
            {oauthModels.map((model) => (
              <SelectItem key={model.index} value={model.model_name}>
                {model.model_name}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
        {oauthModels.length > 0 &&
          (localModels.length > 0 || apiKeyModels.length > 0) && (
            <SelectSeparator />
          )}

        {localModels.length > 0 && (
          <SelectGroup>
            <SelectLabel>{t("chat.modelGroup.local")}</SelectLabel>
            {localModels.map((model) => (
              <SelectItem key={model.index} value={model.model_name}>
                {model.model_name}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
      </SelectContent>
    </Select>
  )
}
