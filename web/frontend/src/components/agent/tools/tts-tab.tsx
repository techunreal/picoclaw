import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  getVoiceConfig,
  testVoice,
  updateVoiceConfig,
  type VoiceConfig,
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
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"
import {
  getSpeechVoices,
  pickSpeechVoice,
  readSpeechVoicePreference,
  speakWithVoice,
  speechSupported,
  stopSpeech,
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
  const [testText, setTestText] = useState("")
  const [audioUrl, setAudioUrl] = useState<string | null>(null)
  const [showAllModels, setShowAllModels] = useState(false)
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>([])
  const [voiceURI, setVoiceURI] = useState("")
  const [browserSpeaking, setBrowserSpeaking] = useState(false)
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

  useEffect(() => {
    return () => {
      if (audioUrl) URL.revokeObjectURL(audioUrl)
    }
  }, [audioUrl])

  const current = draft ?? config?.tts_model_name ?? ""
  const isDirty = config ? current !== config.tts_model_name : false

  const saveMutation = useMutation({
    mutationFn: updateVoiceConfig,
    onSuccess: async (updated: VoiceConfig) => {
      queryClient.setQueryData(queryKey, updated)
      setDraft(null)
      const gateway = await refreshGatewayState({ force: true })
      showSaveSuccessOrRestartToast(
        t,
        t(
          "pages.agent.tools.tts.save_success",
          "Text-to-speech settings saved",
        ),
        t("navigation.tools", "Tools"),
        updated.restart_required === true ||
          gateway?.restartRequired === true,
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.tools.tts.save_error", "Could not save TTS settings"),
      ),
  })

  const testMutation = useMutation({
    mutationFn: testVoice,
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

  const selectBrowserVoice = (value: string) => {
    const uri = value === browserAutoVoice ? "" : value
    setVoiceURI(uri)
    writeSpeechVoicePreference(uri)
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
    const utterance = speakWithVoice(sample, voice, {
      onEnd: () => setBrowserSpeaking(false),
      onError: () => setBrowserSpeaking(false),
    })
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

  return (
    <div className="animate-in fade-in slide-in-from-bottom-2 space-y-6 duration-500">
      {query.isError ? (
        <div className="py-20 text-center">
          <p className="text-destructive font-medium">
            {t("pages.agent.tools.tts.load_error", "Failed to load TTS settings")}
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
                onClick={() => testMutation.mutate(testText)}
                disabled={testMutation.isPending || !config.resolved}
              >
                {testMutation.isPending
                  ? t("pages.agent.tools.tts.testing", "Testing…")
                  : t("pages.agent.tools.tts.test", "Test and play")}
              </Button>
              <Button
                type="button"
                className="h-10 rounded-xl px-6 shadow-sm"
                onClick={() => saveMutation.mutate(current)}
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
                    "Automatic picks the first model whose id contains \"tts\" and has an API key.",
                  )}
                </p>
              </div>
              <div className="w-full space-y-2 sm:w-[420px]">
                <Select
                  value={current === "" ? autoValue : current}
                  onValueChange={(value) =>
                    setDraft(value === autoValue ? "" : value)
                  }
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
                <div className="flex items-center justify-between gap-3">
                  <p className="text-muted-foreground text-[11px] leading-snug">
                    {hasCapableModels
                      ? t(
                          "pages.agent.tools.tts.filter_hint",
                          "Only models that provide /audio/speech are listed (usually the model id contains \"tts\"). Chat models cannot speak.",
                        )
                      : t(
                          "pages.agent.tools.tts.no_tts_models",
                          "None of your models can speak. Add an OpenAI-compatible TTS model (model id usually contains \"tts\") or a MiMo TTS model.",
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
                  "pages.agent.tools.tts.voice_title",
                  "Read aloud voice (WebUI)",
                )}
              </p>
              <p className="text-muted-foreground text-xs">
                {t(
                  "pages.agent.tools.tts.voice_hint",
                  "Used by the speaker button in chats. Automatic follows your browser language; pick a voice to force a language.",
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
        </>
      )}
    </div>
  )
}
