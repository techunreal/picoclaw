import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  clearLocalTTSManifest,
  getLocalTTSModels,
  installLocalTTS,
  saveLocalTTSManifest,
  uninstallLocalTTS,
} from "@/api/local-tts"
import {
  type VoiceCacheStats,
  type VoiceConfig,
  clearVoiceCache,
  getVoiceCacheStats,
  getVoiceConfig,
  testVoice,
  updateVoiceConfig,
} from "@/api/voice"
import { ConfigChangeNotice } from "@/components/config-change-notice"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"
import {
  type ReadAloudEngine,
  type ReadAloudLanguage,
  type ReadAloudSettings,
  getSpeechVoices,
  pickSpeechVoice,
  readReadAloudSettings,
  readSpeechVoicePreference,
  speakWithVoice,
  speechSupported,
  stopSpeech,
  writeReadAloudSettings,
  writeSpeechVoicePreference,
} from "@/lib/speech"
import { refreshGatewayState } from "@/store/gateway"

const queryKey = ["tools", "voice-config"] as const
const autoValue = "__auto__"
const browserAutoVoice = "__auto_voice__"

export function TTSTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<string | null>(null)
  const [voiceDraft, setVoiceDraft] = useState<string | null>(null)
  const [speedDraft, setSpeedDraft] = useState<number | null>(null)
  const [testText, setTestText] = useState("")
  const [audioUrl, setAudioUrl] = useState<string | null>(null)
  const [showAllModels, setShowAllModels] = useState(false)
  const [localUrl, setLocalUrl] = useState("")
  const [pendingInstall, setPendingInstall] = useState<string | null>(null)
  const [acceptLicense, setAcceptLicense] = useState(false)
  const [acceptTools, setAcceptTools] = useState(false)
  const [manifestText, setManifestText] = useState("")
  const [manifestDirty, setManifestDirty] = useState(false)
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>([])
  const [voiceURI, setVoiceURI] = useState("")
  const [browserSpeaking, setBrowserSpeaking] = useState(false)
  const [readAloud, setReadAloud] = useState<ReadAloudSettings>(() =>
    readReadAloudSettings(),
  )
  const audioRef = useRef<HTMLAudioElement | null>(null)

  useEffect(() => {
    if (!speechSupported()) return
    const sync = () => setVoices(getSpeechVoices())
    sync()
    setVoiceURI(readSpeechVoicePreference())
    window.speechSynthesis.addEventListener("voiceschanged", sync)
    return () => {
      window.speechSynthesis.removeEventListener("voiceschanged", sync)
      stopSpeech()
    }
  }, [])

  const query = useQuery({ queryKey, queryFn: getVoiceConfig })
  const config = query.data
  const localQuery = useQuery({
    queryKey: ["tts", "local-models"],
    queryFn: getLocalTTSModels,
    refetchOnWindowFocus: false,
    refetchInterval: (state) =>
      state.state.data?.status.installing ? 1500 : false,
  })
  const local = localQuery.data
  const lastInstalledRef = useRef("")
  const cacheQuery = useQuery({
    queryKey: ["voice", "cache"],
    queryFn: getVoiceCacheStats,
    refetchOnWindowFocus: false,
  })
  const cache: VoiceCacheStats | undefined = cacheQuery.data

  const clearCacheMutation = useMutation({
    mutationFn: clearVoiceCache,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["voice", "cache"] })
      toast.success(
        t("pages.agent.tools.tts.cache_cleared", "Speech cache cleared"),
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.tts.cache_clear_error",
              "Could not clear the speech cache",
            ),
      ),
  })

  useEffect(() => {
    if (!local?.status.done) return
    const id = local.status.model?.recipe_id ?? ""
    if (id === "" || lastInstalledRef.current === id) return
    lastInstalledRef.current = id
    void queryClient.invalidateQueries({ queryKey })
    void queryClient.invalidateQueries({ queryKey: ["models"] })
    void queryClient.invalidateQueries({ queryKey: ["tts", "local-models"] })
  }, [local?.status.done, local?.status.model, queryClient])

  useEffect(() => {
    if (!manifestDirty) setManifestText(local?.user_manifest ?? "")
  }, [local?.user_manifest, manifestDirty])

  useEffect(() => {
    setAcceptLicense(false)
    setAcceptTools(false)
  }, [pendingInstall])

  useEffect(() => {
    if (!config) return
    const speed = config.tts_speed ?? 1
    setReadAloud((current) => {
      if (Math.abs(current.speed - speed) < 0.001) return current
      const next = { ...current, speed }
      writeReadAloudSettings(next)
      return next
    })
  }, [config])

  useEffect(() => {
    return () => {
      if (audioUrl) URL.revokeObjectURL(audioUrl)
    }
  }, [audioUrl])

  const current = draft ?? config?.tts_model_name ?? ""
  const currentVoice = voiceDraft ?? config?.tts_voice ?? ""
  const currentSpeed = speedDraft ?? config?.tts_speed ?? 1
  const showVoiceSelector =
    Boolean(config?.voices?.length) && current === config?.tts_model_name
  const isDirty = config
    ? current !== config.tts_model_name ||
      Math.abs(currentSpeed - (config.tts_speed ?? 1)) > 0.001 ||
      (showVoiceSelector && currentVoice !== (config.tts_voice ?? ""))
    : false

  const saveMutation = useMutation({
    mutationFn: (values: { model: string; voice?: string; speed?: number }) =>
      updateVoiceConfig(values.model, values.voice, values.speed),
    onSuccess: async (updated: VoiceConfig) => {
      queryClient.setQueryData(queryKey, updated)
      setDraft(null)
      setVoiceDraft(null)
      setSpeedDraft(null)
      const settings = readReadAloudSettings()
      writeReadAloudSettings({
        ...settings,
        speed: updated.tts_speed ?? 1,
      })
      const gateway = await refreshGatewayState({ force: true })
      showSaveSuccessOrRestartToast(
        t,
        t(
          "pages.agent.tools.tts.save_success",
          "Text-to-speech settings saved",
        ),
        t("navigation.tools", "Tools"),
        updated.restart_required === true || gateway?.restartRequired === true,
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.tts.save_error",
              "Could not save TTS settings",
            ),
      ),
  })

  const installMutation = useMutation({
    mutationFn: (values: {
      url: string
      acceptLicense?: string
      acceptTools?: boolean
    }) =>
      installLocalTTS(
        values.url,
        true,
        values.acceptLicense,
        values.acceptTools,
      ),
    onSuccess: async () => {
      setPendingInstall(null)
      toast.success(
        t(
          "pages.agent.tools.tts.local_install_started",
          "Local model installation started",
        ),
      )
      await queryClient.invalidateQueries({
        queryKey: ["tts", "local-models"],
      })
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.tts.local_install_error",
              "Could not start the local model installation",
            ),
      ),
  })

  const uninstallMutation = useMutation({
    mutationFn: (id: string) => uninstallLocalTTS(id),
    onSuccess: async () => {
      toast.success(
        t("pages.agent.tools.tts.local_uninstalled", "Local model removed"),
      )
      await queryClient.invalidateQueries({
        queryKey: ["tts", "local-models"],
      })
      await queryClient.invalidateQueries({ queryKey })
      await queryClient.invalidateQueries({ queryKey: ["models"] })
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.tts.local_uninstall_error",
              "Could not remove the model",
            ),
      ),
  })

  const manifestMutation = useMutation({
    mutationFn: async (raw: string) => {
      let parsed: unknown
      try {
        parsed = JSON.parse(raw)
      } catch (error) {
        throw new Error(
          error instanceof Error
            ? error.message
            : t(
                "pages.agent.tools.tts.local_manifest_invalid_json",
                "The manifest is not valid JSON.",
              ),
          { cause: error },
        )
      }
      return saveLocalTTSManifest(parsed)
    },
    onSuccess: async () => {
      setManifestDirty(false)
      toast.success(
        t(
          "pages.agent.tools.tts.local_manifest_saved",
          "Third-party manifest saved",
        ),
      )
      await queryClient.invalidateQueries({
        queryKey: ["tts", "local-models"],
      })
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.tts.local_manifest_error",
              "Could not save the manifest",
            ),
      ),
  })

  const clearManifestMutation = useMutation({
    mutationFn: clearLocalTTSManifest,
    onSuccess: async () => {
      setManifestText("")
      setManifestDirty(false)
      toast.success(
        t(
          "pages.agent.tools.tts.local_manifest_cleared",
          "Third-party manifest removed",
        ),
      )
      await queryClient.invalidateQueries({
        queryKey: ["tts", "local-models"],
      })
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.tts.local_manifest_error",
              "Could not remove the manifest",
            ),
      ),
  })

  const testMutation = useMutation({
    mutationFn: (values: {
      text: string
      model?: string
      voice?: string
      speed?: number
    }) => testVoice(values.text, values.model, values.voice, values.speed),
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob)
      setAudioUrl((previous) => {
        if (previous) URL.revokeObjectURL(previous)
        return url
      })
      window.setTimeout(() => audioRef.current?.play().catch(() => {}), 50)
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.tools.tts.test_error", "TTS test failed"),
      ),
  })

  const selectedVoiceAvailable = voices.some(
    (voice) => voice.voiceURI === voiceURI,
  )
  const voiceSelectValue = selectedVoiceAvailable ? voiceURI : browserAutoVoice

  const ttsTestSample = () => {
    const selected = selectedVoiceAvailable
      ? (voices.find((item) => item.voiceURI === voiceURI) ?? null)
      : pickSpeechVoice()
    const lang = (selected?.lang || navigator.language || "en").toLowerCase()
    return lang.startsWith("tr")
      ? "Merhaba, bu bir ses testidir."
      : t("pages.agent.tools.tts.voice_sample", "Hello, this is a speech test.")
  }

  const selectBrowserVoice = (value: string) => {
    const uri = value === browserAutoVoice ? "" : value
    setVoiceURI(uri)
    writeSpeechVoicePreference(uri)
  }

  const updateReadAloud = (patch: Partial<ReadAloudSettings>) => {
    setReadAloud((current) => {
      const next = { ...current, ...patch }
      writeReadAloudSettings(next)
      return next
    })
  }

  const testBrowserVoice = () => {
    if (browserSpeaking) {
      stopSpeech()
      setBrowserSpeaking(false)
      return
    }
    const voice = selectedVoiceAvailable
      ? (voices.find((item) => item.voiceURI === voiceURI) ?? null)
      : pickSpeechVoice()
    const lang = (voice?.lang || navigator.language || "en").toLowerCase()
    const sample = lang.startsWith("tr")
      ? "Merhaba, bu bir ses testidir."
      : t("pages.agent.tools.tts.voice_sample", "Hello, this is a speech test.")
    const utterance = speakWithVoice(
      sample,
      voice,
      {
        onEnd: () => setBrowserSpeaking(false),
        onError: () => setBrowserSpeaking(false),
      },
      "",
      currentSpeed,
    )
    setBrowserSpeaking(Boolean(utterance))
  }

  const options = useMemo(() => {
    const allModels = config?.models ?? []
    const capable = (model: (typeof allModels)[number]) =>
      model.tts_like || model.provider === "mimo"
    const models = showAllModels
      ? allModels
      : allModels.filter((model) => capable(model) || model.name === current)
    const known = models.some((model) => model.name === current)
    if (current !== "" && !known) {
      return [
        {
          name: current,
          provider: "",
          model: "",
          has_key: false,
          tts_like: false,
          missing: true,
          capable: false,
        },
        ...models.map((model) => ({
          ...model,
          missing: false,
          capable: capable(model),
        })),
      ]
    }
    return models.map((model) => ({
      ...model,
      missing: false,
      capable: capable(model),
    }))
  }, [config?.models, current, showAllModels])

  const hasCapableModels = (config?.models ?? []).some(
    (model) => model.tts_like || model.provider === "mimo",
  )

  const pendingKey = (pendingInstall ?? "")
    .trim()
    .toLowerCase()
    .replace(/\/+$/, "")
  const pendingItem =
    local?.catalog.find((item) => {
      if (pendingKey === "") return false
      const id = item.id.toLowerCase()
      const repo = item.hf_repo?.toLowerCase() ?? ""
      return (
        pendingKey === id ||
        (repo !== "" &&
          (pendingKey === repo ||
            pendingKey.includes(`huggingface.co/${repo}`)))
      )
    }) ?? null
  const pendingThirdParty = Boolean(pendingItem?.third_party)
  const pendingMissing = pendingItem?.missing ?? []
  const pendingTool =
    pendingItem?.dependencies?.find(
      (dependency) => !dependency.available && dependency.can_install,
    ) ?? null

  return (
    <div className="animate-in fade-in slide-in-from-bottom-2 space-y-6 duration-500">
      {query.isError ? (
        <div className="py-20 text-center">
          <p className="text-destructive font-medium">
            {t(
              "pages.agent.tools.tts.load_error",
              "Failed to load TTS settings",
            )}
          </p>
        </div>
      ) : query.isLoading || !config ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full rounded-2xl" />
          <Skeleton className="h-64 w-full rounded-2xl" />
        </div>
      ) : (
        <>
          <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
            <div className="max-w-xl space-y-3">
              <h1 className="text-foreground/90 text-2xl font-semibold tracking-tight">
                {t("pages.agent.tools.tts.title", "Text to Speech")}
              </h1>
              <p className="text-muted-foreground/80 text-[14px] leading-relaxed">
                {t(
                  "pages.agent.tools.tts.description",
                  "Choose which model speaks the agent's replies. The send_tts tool stays available once a TTS model is ready.",
                )}
              </p>
            </div>

            <div className="flex shrink-0 gap-2">
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-xl px-4"
                onClick={() =>
                  testMutation.mutate({
                    text: testText.trim() || ttsTestSample(),
                    model: current === "" ? undefined : current,
                    voice: showVoiceSelector ? currentVoice : undefined,
                    speed: currentSpeed,
                  })
                }
                disabled={
                  testMutation.isPending || (!config.resolved && current === "")
                }
              >
                {testMutation.isPending
                  ? t("pages.agent.tools.tts.testing", "Testing…")
                  : t("pages.agent.tools.tts.test", "Test and play")}
              </Button>
              <Button
                type="button"
                className="h-10 rounded-xl px-6 shadow-sm"
                onClick={() =>
                  saveMutation.mutate({
                    model: current,
                    voice: showVoiceSelector ? currentVoice : undefined,
                    speed: currentSpeed,
                  })
                }
                disabled={!isDirty || saveMutation.isPending}
              >
                {t("pages.agent.tools.tts.save", "Save Changes")}
              </Button>
            </div>
          </div>

          {isDirty && (
            <ConfigChangeNotice
              kind="save"
              title={t("pages.agent.tools.tts.unsaved", "Unsaved changes")}
              description={t(
                "pages.agent.tools.tts.unsaved_description",
                "Saving writes voice.tts_model_name to config.json and restarts the gateway.",
              )}
            />
          )}

          <div className="bg-card border-border/40 divide-border/40 divide-y overflow-hidden rounded-2xl border shadow-sm">
            <div className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t("pages.agent.tools.tts.model", "TTS model")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t(
                    "pages.agent.tools.tts.model_hint",
                    'Automatic picks the first model whose id contains "tts" and has an API key.',
                  )}
                </p>
              </div>
              <div className="w-full space-y-2 sm:w-[420px]">
                <Select
                  value={current === "" ? autoValue : current}
                  onValueChange={(value) => {
                    setDraft(value === autoValue ? "" : value)
                    setVoiceDraft(null)
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={autoValue}>
                      {t("pages.agent.tools.tts.automatic", "Automatic")}
                    </SelectItem>
                    {options.map((option) => (
                      <SelectItem
                        key={option.name}
                        value={option.name}
                        disabled={option.missing}
                      >
                        {option.name}
                        {option.provider
                          ? ` — ${option.provider}/${option.model}`
                          : ""}
                        {option.missing
                          ? ` ${t("pages.agent.tools.tts.missing", "(not found)")}`
                          : ""}
                        {!option.has_key && !option.missing
                          ? ` ${t("pages.agent.tools.tts.no_key", "(no API key)")}`
                          : ""}
                        {!option.capable && !option.missing
                          ? ` ${t("pages.agent.tools.tts.chat_model", "(chat model, cannot speak)")}`
                          : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {showVoiceSelector && (
                  <Select
                    value={currentVoice || config.voices?.[0]?.id || ""}
                    onValueChange={setVoiceDraft}
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue
                        placeholder={t("pages.agent.tools.tts.voice", "Voice")}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {config.voices?.map((voice) => (
                        <SelectItem key={voice.id} value={voice.id}>
                          {voice.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                <div className="space-y-1">
                  <div className="flex items-center justify-between">
                    <p className="text-sm font-medium">
                      {t("pages.agent.tools.tts.speed", "Speech speed")}
                    </p>
                    <span className="text-muted-foreground text-xs">
                      {currentSpeed.toFixed(1)}×
                    </span>
                  </div>
                  <input
                    type="range"
                    min={0.5}
                    max={2}
                    step={0.1}
                    value={currentSpeed}
                    onChange={(event) =>
                      setSpeedDraft(Number(event.target.value))
                    }
                    className="accent-primary w-full"
                    aria-label={t(
                      "pages.agent.tools.tts.speed",
                      "Speech speed",
                    )}
                  />
                  <p className="text-muted-foreground text-[11px] leading-snug">
                    {t(
                      "pages.agent.tools.tts.speed_hint",
                      "0.5× is slower, 2.0× is faster. Used by read-aloud, tests and send_tts.",
                    )}
                  </p>
                </div>
                <div className="flex items-center justify-between gap-3">
                  <p className="text-muted-foreground text-[11px] leading-snug">
                    {hasCapableModels
                      ? t(
                          "pages.agent.tools.tts.filter_hint",
                          'Only models that provide /audio/speech are listed (usually the model id contains "tts"). Chat models cannot speak.',
                        )
                      : t(
                          "pages.agent.tools.tts.no_tts_models",
                          'None of your models can speak. Add an OpenAI-compatible TTS model (model id usually contains "tts") or a MiMo TTS model.',
                        )}
                  </p>
                  <Button
                    type="button"
                    variant="link"
                    size="sm"
                    className="h-auto shrink-0 px-0 text-[11px]"
                    onClick={() => setShowAllModels((value) => !value)}
                  >
                    {showAllModels
                      ? t("pages.agent.tools.tts.hide_all", "Hide chat models")
                      : t("pages.agent.tools.tts.show_all", "Show chat models")}
                  </Button>
                </div>
              </div>
            </div>

            <div className="p-5">
              <p className="text-sm font-medium">
                {t("pages.agent.tools.tts.effective", "Effective provider")}
              </p>
              {config.resolved ? (
                <p className="text-muted-foreground mt-1 text-xs">
                  {config.resolved_name} — {config.resolved_provider}/
                  {config.resolved_model}
                </p>
              ) : (
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  {t(
                    "pages.agent.tools.tts.not_ready",
                    "No TTS model is ready yet. Add one in Models with an API key, then select it here.",
                  )}
                </p>
              )}
            </div>

            <div className="space-y-3 p-5">
              <p className="text-sm font-medium">
                {t("pages.agent.tools.tts.test_title", "Try it")}
              </p>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Input
                  value={testText}
                  onChange={(event) => setTestText(event.target.value)}
                  placeholder={t(
                    "pages.agent.tools.tts.test_placeholder",
                    "Leave empty to use the sample sentence",
                  )}
                  maxLength={400}
                />
              </div>
              {audioUrl && (
                <audio
                  ref={audioRef}
                  controls
                  preload="metadata"
                  src={audioUrl}
                  className="w-full max-w-md"
                />
              )}
            </div>
          </div>

          <div className="bg-card border-border/40 space-y-4 rounded-2xl border p-5 shadow-sm">
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {t(
                  "pages.agent.tools.tts.read_aloud_title",
                  "Read aloud (WebUI)",
                )}
              </p>
              <p className="text-muted-foreground text-xs">
                {t(
                  "pages.agent.tools.tts.read_aloud_hint",
                  "Used by the speaker button in chats. Automatic uses the selected TTS model when it is ready, otherwise the browser voice.",
                )}
              </p>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <p className="text-sm font-medium">
                  {t("pages.agent.tools.tts.read_engine", "Engine")}
                </p>
                <Select
                  value={readAloud.engine}
                  onValueChange={(value) =>
                    updateReadAloud({ engine: value as ReadAloudEngine })
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">
                      {t(
                        "pages.agent.tools.tts.read_engine_auto",
                        "Automatic (TTS model, then browser)",
                      )}
                    </SelectItem>
                    <SelectItem value="tts">
                      {t(
                        "pages.agent.tools.tts.read_engine_tts",
                        "PicoClaw TTS model",
                      )}
                    </SelectItem>
                    <SelectItem value="browser">
                      {t(
                        "pages.agent.tools.tts.read_engine_browser",
                        "Browser voice",
                      )}
                    </SelectItem>
                  </SelectContent>
                </Select>
                {readAloud.engine === "tts" && !config.resolved && (
                  <p className="text-xs text-amber-600 dark:text-amber-400">
                    {t(
                      "pages.agent.tools.tts.read_tts_unavailable",
                      "No TTS model is ready. Select one above or use Automatic.",
                    )}
                  </p>
                )}
              </div>
              <div className="space-y-2">
                <p className="text-sm font-medium">
                  {t("pages.agent.tools.tts.read_language", "Language")}
                </p>
                <Select
                  value={readAloud.language}
                  onValueChange={(value) =>
                    updateReadAloud({ language: value as ReadAloudLanguage })
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">
                      {t(
                        "pages.agent.tools.tts.read_language_auto",
                        "Automatic (detected)",
                      )}
                    </SelectItem>
                    <SelectItem value="tr">
                      {t("pages.agent.tools.tts.read_language_tr", "Turkish")}
                    </SelectItem>
                    <SelectItem value="en">
                      {t("pages.agent.tools.tts.read_language_en", "English")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t(
                    "pages.agent.tools.tts.read_auto_language",
                    "Detect language automatically",
                  )}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t(
                    "pages.agent.tools.tts.read_auto_language_hint",
                    "A manual language always wins. Turn this off to always use the language selected above.",
                  )}
                </p>
              </div>
              <Switch
                checked={readAloud.autoLanguage}
                onCheckedChange={(checked) => {
                  const fallbackLanguage: ReadAloudLanguage = navigator.language
                    .toLowerCase()
                    .startsWith("tr")
                    ? "tr"
                    : "en"
                  updateReadAloud({
                    autoLanguage: checked,
                    language:
                      checked || readAloud.language !== "auto"
                        ? readAloud.language
                        : fallbackLanguage,
                  })
                }}
              />
            </div>
          </div>

          <div className="bg-card border-border/40 space-y-4 rounded-2xl border p-5 shadow-sm">
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {t(
                  "pages.agent.tools.tts.voice_title",
                  "Browser fallback voice",
                )}
              </p>
              <p className="text-muted-foreground text-xs">
                {t(
                  "pages.agent.tools.tts.voice_hint",
                  "Used when read-aloud falls back to the browser. Automatic follows the message language first, then the browser language.",
                )}
              </p>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Select
                value={voiceSelectValue}
                onValueChange={selectBrowserVoice}
              >
                <SelectTrigger className="w-full sm:w-96">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={browserAutoVoice}>
                    {t(
                      "pages.agent.tools.tts.voice_auto",
                      "Automatic (browser language)",
                    )}
                  </SelectItem>
                  {voices.map((voice) => (
                    <SelectItem key={voice.voiceURI} value={voice.voiceURI}>
                      {voice.name} — {voice.lang}
                      {voice.default ? " ★" : ""}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-xl px-4"
                onClick={testBrowserVoice}
                disabled={voices.length === 0}
              >
                {browserSpeaking
                  ? t("pages.agent.tools.tts.voice_stop", "Stop")
                  : t("pages.agent.tools.tts.voice_test", "Test voice")}
              </Button>
            </div>
            {voices.length === 0 && (
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {t(
                  "pages.agent.tools.tts.voice_none",
                  "This browser did not expose any speech voices. Edge or Chrome usually provides them.",
                )}
              </p>
            )}
          </div>

          <div className="bg-card border-border/40 space-y-3 rounded-2xl border p-5 shadow-sm">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t("pages.agent.tools.tts.cache_title", "Speech cache")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t(
                    "pages.agent.tools.tts.cache_hint",
                    "Generated audio is reused, so replaying the same reply is instant. Files expire after 1 hour and the folder is capped at 200 MB.",
                  )}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="shrink-0"
                onClick={() => clearCacheMutation.mutate()}
                disabled={
                  clearCacheMutation.isPending || (cache?.entries ?? 0) === 0
                }
              >
                {clearCacheMutation.isPending
                  ? t("pages.agent.tools.tts.cache_clearing", "Clearing…")
                  : t("pages.agent.tools.tts.cache_clear", "Clear cache")}
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">
              {cache
                ? `${cache.entries} ${t("pages.agent.tools.tts.cache_files", "files")} · ${(
                    cache.bytes /
                    (1024 * 1024)
                  ).toFixed(1)} MB / ${(
                    cache.max_bytes /
                    (1024 * 1024)
                  ).toFixed(0)} MB`
                : t("labels.loading", "Loading…")}
            </p>
          </div>

          <div className="bg-muted/30 border-border/40 space-y-2 rounded-2xl border p-5">
            <p className="text-sm font-medium">
              {t("pages.agent.tools.tts.local_title", "Local TTS")}
            </p>
            <p className="text-muted-foreground text-xs leading-relaxed">
              {t(
                "pages.agent.tools.tts.local_hint",
                "You can run TTS on your own machine: add a model with provider openai-compatible, base URL such as http://127.0.0.1:8000/v1 (for example openedai-speech or Kokoro) and a placeholder API key, then select it here. No cloud key is needed.",
              )}
            </p>
            <p className="text-muted-foreground text-xs leading-relaxed">
              {t(
                "pages.agent.tools.tts.channel_hint",
                "Telegram receives the audio as a voice message. In the WebUI the reply shows a player inside the chat bubble.",
              )}
            </p>
          </div>

          <div className="bg-card border-border/40 space-y-4 rounded-2xl border p-5 shadow-sm">
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {t(
                  "pages.agent.tools.tts.local_models_title",
                  "Local models (on demand)",
                )}
              </p>
              <p className="text-muted-foreground text-xs">
                {t(
                  "pages.agent.tools.tts.local_models_hint",
                  "Models are downloaded to ~/.picoclaw/tts only when you install them; the ZIP stays small. A small Python sidecar runs them on 127.0.0.1.",
                )}
              </p>
            </div>

            {localQuery.isLoading ? (
              <p className="text-muted-foreground text-xs">
                {t("labels.loading", "Loading…")}
              </p>
            ) : localQuery.isError ? (
              <p className="text-destructive text-xs">
                {t(
                  "pages.agent.tools.tts.local_load_error",
                  "Could not load local model information.",
                )}
              </p>
            ) : local ? (
              <>
                {!local.environment.available && (
                  <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
                    <p className="font-medium">
                      {t(
                        "pages.agent.tools.tts.python_required",
                        "Python 3.10+ is required",
                      )}
                    </p>
                    <p className="text-muted-foreground mt-1">
                      {local.environment.message ||
                        t(
                          "pages.agent.tools.tts.python_missing",
                          "Install Python with pip and venv support, then refresh this page.",
                        )}
                    </p>
                    <a
                      className="text-primary mt-1 inline-block underline"
                      href="https://www.python.org/downloads/"
                      target="_blank"
                      rel="noreferrer"
                    >
                      python.org/downloads
                    </a>
                  </div>
                )}

                <div className="flex flex-col gap-2 sm:flex-row">
                  <Input
                    value={localUrl}
                    onChange={(event) => setLocalUrl(event.target.value)}
                    placeholder={t(
                      "pages.agent.tools.tts.local_url_placeholder",
                      "Hugging Face URL or model id (e.g. wite-tech/clarvia-tts-tiny)",
                    )}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    className="h-10 shrink-0 rounded-xl px-4"
                    onClick={() => {
                      const value = localUrl.trim()
                      if (value === "") return
                      if (!local.environment.available) {
                        toast.error(
                          t(
                            "pages.agent.tools.tts.python_required",
                            "Python 3.10+ is required",
                          ),
                        )
                        return
                      }
                      setPendingInstall(value)
                    }}
                  >
                    {t("pages.agent.tools.tts.local_review", "Review install")}
                  </Button>
                </div>

                {pendingInstall && (
                  <div className="border-primary/30 bg-primary/5 rounded-lg border p-3 text-xs">
                    <p className="font-medium">
                      {t(
                        "pages.agent.tools.tts.local_confirm_title",
                        "Install this local model?",
                      )}
                    </p>
                    <p className="text-muted-foreground mt-1">
                      {t(
                        "pages.agent.tools.tts.local_confirm_body",
                        "PicoClaw will create a Python virtual environment under ~/.picoclaw/tts, download the model files and start a local sidecar. This uses disk and network.",
                      )}
                    </p>
                    <p className="text-muted-foreground mt-1">
                      {t("pages.agent.tools.tts.local_python_path", "Python:")}{" "}
                      {local.environment.executable}{" "}
                      {local.environment.version
                        ? `(${local.environment.version})`
                        : ""}
                    </p>
                    {pendingThirdParty && pendingItem && (
                      <div className="mt-2 space-y-1 rounded-md border border-amber-500/40 bg-amber-500/5 p-2">
                        <p className="font-medium text-amber-700 dark:text-amber-300">
                          {t(
                            "pages.agent.tools.tts.local_third_party_title",
                            "Third-party model",
                          )}
                        </p>
                        <p className="text-muted-foreground">
                          {t("pages.agent.tools.tts.local_license", "License")}:{" "}
                          {pendingItem.license_url ? (
                            <a
                              className="text-primary underline"
                              href={pendingItem.license_url}
                              target="_blank"
                              rel="noreferrer"
                            >
                              {pendingItem.license}
                            </a>
                          ) : (
                            pendingItem.license
                          )}
                          {pendingItem.source_url ? (
                            <>
                              {" · "}
                              <a
                                className="text-primary underline"
                                href={pendingItem.source_url}
                                target="_blank"
                                rel="noreferrer"
                              >
                                {t(
                                  "pages.agent.tools.tts.local_source",
                                  "Source",
                                )}
                              </a>
                            </>
                          ) : null}
                        </p>
                        <label className="flex items-start gap-2">
                          <input
                            type="checkbox"
                            className="mt-0.5"
                            checked={acceptLicense}
                            onChange={(event) =>
                              setAcceptLicense(event.target.checked)
                            }
                          />
                          <span>
                            {t(
                              "pages.agent.tools.tts.local_accept_license",
                              "I have read the model license and accept its terms.",
                            )}
                          </span>
                        </label>
                      </div>
                    )}
                    {pendingTool && (
                      <div className="mt-2 space-y-1 rounded-md border border-sky-500/40 bg-sky-500/5 p-2">
                        <p className="font-medium text-sky-700 dark:text-sky-300">
                          {t(
                            "pages.agent.tools.tts.local_tool_title",
                            "eSpeak NG is required",
                          )}
                        </p>
                        <p className="text-muted-foreground">
                          {t(
                            "pages.agent.tools.tts.local_tool_body",
                            "PicoClaw can download eSpeak NG 1.52.0 (GPL-3.0, about 12 MB) to ~/.picoclaw/tts/tools. It is not bundled with the ZIP.",
                          )}
                        </p>
                        <label className="flex items-start gap-2">
                          <input
                            type="checkbox"
                            className="mt-0.5"
                            checked={acceptTools}
                            onChange={(event) =>
                              setAcceptTools(event.target.checked)
                            }
                          />
                          <span>
                            {t(
                              "pages.agent.tools.tts.local_tool_accept",
                              "I accept the eSpeak NG license (GPL-3.0).",
                            )}
                          </span>
                        </label>
                      </div>
                    )}
                    {pendingMissing.length > 0 && !pendingTool && (
                      <p className="text-destructive mt-2">
                        {t(
                          "pages.agent.tools.tts.local_missing_dependency",
                          "Install the required tool first:",
                        )}{" "}
                        {pendingMissing.join(", ")}
                      </p>
                    )}
                    <div className="mt-2 flex gap-2">
                      <Button
                        type="button"
                        size="sm"
                        onClick={() =>
                          installMutation.mutate({
                            url: pendingInstall,
                            acceptLicense:
                              pendingThirdParty && pendingInstall
                                ? pendingInstall
                                : undefined,
                            acceptTools: pendingTool ? acceptTools : undefined,
                          })
                        }
                        disabled={
                          installMutation.isPending ||
                          (pendingMissing.length > 0 && !pendingTool) ||
                          (pendingTool !== null && !acceptTools) ||
                          (pendingThirdParty && !acceptLicense)
                        }
                      >
                        {installMutation.isPending
                          ? t(
                              "pages.agent.tools.tts.local_installing",
                              "Starting…",
                            )
                          : t("pages.agent.tools.tts.local_install", "Install")}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => setPendingInstall(null)}
                        disabled={installMutation.isPending}
                      >
                        {t("pages.agent.tools.tts.local_cancel", "Cancel")}
                      </Button>
                    </div>
                  </div>
                )}

                {local.status.installing && (
                  <div className="rounded-lg border p-3 text-xs">
                    <p className="font-medium">
                      {local.status.message ||
                        t(
                          "pages.agent.tools.tts.local_working",
                          "Installing local model…",
                        )}
                    </p>
                    <p className="text-muted-foreground mt-1">
                      {local.status.phase} · {local.status.progress}%
                    </p>
                  </div>
                )}

                {local.status.error && (
                  <p className="text-destructive text-xs">
                    {local.status.error}
                  </p>
                )}

                <div className="grid gap-2">
                  {local.catalog.map((item) => (
                    <div
                      key={item.id}
                      className="flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="space-y-1">
                        <p className="text-sm font-medium">
                          {item.name}
                          {item.third_party ? (
                            <span className="ml-2 rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-[10px] font-normal text-amber-700 dark:text-amber-300">
                              {t(
                                "pages.agent.tools.tts.local_third_party_badge",
                                "Third-party",
                              )}
                            </span>
                          ) : null}
                        </p>
                        <p className="text-muted-foreground text-xs">
                          {item.language.toUpperCase()} · {item.license} ·{" "}
                          {Math.round(item.size_bytes / (1024 * 1024))} MB ·{" "}
                          {item.voices.length}{" "}
                          {t("pages.agent.tools.tts.local_voices", "voices")}
                          {item.source_url ? (
                            <>
                              {" · "}
                              <a
                                className="text-primary underline"
                                href={item.source_url}
                                target="_blank"
                                rel="noreferrer"
                              >
                                {t(
                                  "pages.agent.tools.tts.local_source",
                                  "Source",
                                )}
                              </a>
                            </>
                          ) : null}
                        </p>
                        {item.installed && (
                          <p className="text-xs text-emerald-600 dark:text-emerald-400">
                            {item.running
                              ? t(
                                  "pages.agent.tools.tts.local_running",
                                  "Installed · sidecar running",
                                )
                              : (local.idle_timeout_minutes ?? 0) > 0
                                ? t(
                                    "pages.agent.tools.tts.local_sleeping",
                                    "Installed · sleeping",
                                  )
                                : t(
                                    "pages.agent.tools.tts.local_installed",
                                    "Installed",
                                  )}
                          </p>
                        )}
                        {!item.installed && (item.missing?.length ?? 0) > 0 && (
                          <p className="text-muted-foreground text-xs">
                            {t(
                              "pages.agent.tools.tts.local_requires_tool",
                              "Requires:",
                            )}{" "}
                            {item.missing?.join(", ")}
                          </p>
                        )}
                      </div>
                      <div className="flex shrink-0 gap-2">
                        {item.installed ? (
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            onClick={() => {
                              if (
                                window.confirm(
                                  t(
                                    "pages.agent.tools.tts.local_uninstall_confirm",
                                    "Remove this local model and its files?",
                                  ),
                                )
                              ) {
                                uninstallMutation.mutate(item.id)
                              }
                            }}
                            disabled={uninstallMutation.isPending}
                          >
                            {t("pages.agent.tools.tts.local_remove", "Remove")}
                          </Button>
                        ) : (
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => {
                              if (!local.environment.available) {
                                toast.error(
                                  t(
                                    "pages.agent.tools.tts.python_required",
                                    "Python 3.10+ is required",
                                  ),
                                )
                                return
                              }
                              setPendingInstall(item.id)
                            }}
                          >
                            {t(
                              "pages.agent.tools.tts.local_install",
                              "Install",
                            )}
                          </Button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>

                <details className="rounded-lg border p-3">
                  <summary className="cursor-pointer text-sm font-medium">
                    {t(
                      "pages.agent.tools.tts.local_manifest_title",
                      "Advanced: third-party manifest",
                    )}
                  </summary>
                  <div className="mt-2 space-y-2">
                    <p className="text-muted-foreground text-xs">
                      {t(
                        "pages.agent.tools.tts.local_manifest_hint",
                        "For advanced users. Add a model that uses a supported front end (for example another eSpeak-based tiny-voca fork). The manifest only describes files, license and voices; no model code is executed.",
                      )}
                    </p>
                    {local.manifest_error && (
                      <p className="text-destructive text-xs">
                        {local.manifest_error}
                      </p>
                    )}
                    <textarea
                      className="border-input bg-background focus-visible:ring-ring min-h-40 w-full rounded-lg border p-3 font-mono text-xs outline-none focus-visible:ring-2"
                      value={manifestText}
                      spellCheck={false}
                      onChange={(event) => {
                        setManifestText(event.target.value)
                        setManifestDirty(true)
                      }}
                      placeholder={t(
                        "pages.agent.tools.tts.local_manifest_placeholder",
                        '{ "version": 1, "models": [ { "id": "...", "third_party": true, ... } ] }',
                      )}
                    />
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="sm"
                        onClick={() => manifestMutation.mutate(manifestText)}
                        disabled={
                          manifestMutation.isPending ||
                          manifestText.trim() === ""
                        }
                      >
                        {manifestMutation.isPending
                          ? t(
                              "pages.agent.tools.tts.local_manifest_saving",
                              "Saving…",
                            )
                          : t(
                              "pages.agent.tools.tts.local_manifest_save",
                              "Save manifest",
                            )}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setManifestText(local.user_manifest ?? "")
                          setManifestDirty(false)
                        }}
                        disabled={!manifestDirty}
                      >
                        {t(
                          "pages.agent.tools.tts.local_manifest_reset",
                          "Reset",
                        )}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          if (
                            window.confirm(
                              t(
                                "pages.agent.tools.tts.local_manifest_remove_confirm",
                                "Remove the saved third-party manifest?",
                              ),
                            )
                          ) {
                            clearManifestMutation.mutate()
                          }
                        }}
                        disabled={
                          clearManifestMutation.isPending ||
                          !local.user_manifest
                        }
                      >
                        {t(
                          "pages.agent.tools.tts.local_manifest_remove",
                          "Remove saved",
                        )}
                      </Button>
                    </div>
                  </div>
                </details>
              </>
            ) : null}
          </div>
        </>
      )}
    </div>
  )
}
