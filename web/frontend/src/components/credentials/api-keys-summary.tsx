import { IconArrowRight, IconKey, IconLoader2 } from "@tabler/icons-react"
import { Link } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { type ModelInfo, getModels } from "@/api/models"
import { Button } from "@/components/ui/button"

function modelKeyCount(model: ModelInfo): number {
  if (typeof model.api_key_count === "number") return model.api_key_count
  if (model.api_keys && model.api_keys.length > 0) return model.api_keys.length
  return model.api_key ? 1 : 0
}

export function ApiKeysSummary() {
  const { t } = useTranslation()
  const [models, setModels] = useState<ModelInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  useEffect(() => {
    let canceled = false
    getModels()
      .then((data) => {
        if (!canceled) setModels(data.models || [])
      })
      .catch((err) => {
        if (!canceled) {
          setError(
            err instanceof Error
              ? err.message
              : t("credentials.apiKeys.loadFailed"),
          )
        }
      })
      .finally(() => {
        if (!canceled) setLoading(false)
      })
    return () => {
      canceled = true
    }
  }, [t])

  const modelsWithKeys = models.filter(
    (model) => !model.is_virtual && modelKeyCount(model) > 0,
  )
  const totalKeys = modelsWithKeys.reduce(
    (sum, model) => sum + modelKeyCount(model),
    0,
  )

  return (
    <section className="bg-card mt-4 rounded-xl border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            <IconKey className="size-4" />
            {t("credentials.apiKeys.title")}
          </h2>
          <p className="text-muted-foreground mt-1 text-sm">
            {t("credentials.apiKeys.description")}
          </p>
        </div>
        <Button asChild size="sm" variant="outline">
          <Link to="/models">
            {t("credentials.apiKeys.manage")}
            <IconArrowRight className="size-3.5" />
          </Link>
        </Button>
      </div>

      {loading && (
        <div className="text-muted-foreground mt-4 flex items-center gap-2 text-sm">
          <IconLoader2 className="size-4 animate-spin" />
          {t("credentials.apiKeys.loading")}
        </div>
      )}

      {!loading && error && (
        <p className="text-destructive mt-4 text-sm">{error}</p>
      )}

      {!loading && !error && modelsWithKeys.length === 0 && (
        <p className="text-muted-foreground mt-4 text-sm">
          {t("credentials.apiKeys.empty")}
        </p>
      )}

      {!loading && !error && modelsWithKeys.length > 0 && (
        <>
          <p className="text-muted-foreground mt-3 text-xs">
            {t("credentials.apiKeys.summary", {
              models: modelsWithKeys.length,
              keys: totalKeys,
            })}
          </p>
          <div className="mt-2 space-y-1.5">
            {modelsWithKeys.map((model) => {
              const masks =
                model.api_keys && model.api_keys.length > 0
                  ? model.api_keys
                  : [model.api_key].filter(Boolean)
              return (
                <div
                  key={`${model.index}:${model.model_name}`}
                  className="bg-muted/30 flex flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2"
                >
                  <div className="min-w-0">
                    <span className="text-sm font-medium">
                      {model.model_name}
                    </span>
                    {model.provider && (
                      <span className="text-muted-foreground ml-2 font-mono text-xs">
                        {model.provider}
                      </span>
                    )}
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5">
                    {masks.map((mask, index) => (
                      <code
                        key={index}
                        className="bg-background rounded border px-1.5 py-0.5 font-mono text-[11px]"
                      >
                        {mask}
                      </code>
                    ))}
                    {!model.enabled && (
                      <span className="bg-destructive/10 text-destructive rounded px-1.5 py-0.5 text-[10px] font-medium">
                        {t("models.badge.disabled")}
                      </span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </>
      )}
    </section>
  )
}
