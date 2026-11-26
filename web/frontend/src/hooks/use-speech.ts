import { useCallback, useEffect, useRef, useState } from "react"

import { speakVoice } from "@/api/voice"
import {
  pickSpeechVoice,
  readReadAloudSettings,
  resolveReadAloudLanguage,
  speechSupported,
  speakWithVoice,
  stopSpeech,
} from "@/lib/speech"

function stripMarkdown(text: string): string {
  return text
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    // Emoji, dingbats, variation selectors and zero-width joiners: never read aloud.
    .replace(/\p{Extended_Pictographic}(\uFE0F|\u200D\p{Extended_Pictographic})*/gu, " ")
    .replace(/[\u{1F000}-\u{1FAFF}]/gu, " ")
    .replace(/[\u{2600}-\u{27BF}]/gu, " ")
    .replace(/[\u{2B00}-\u{2BFF}]/gu, " ")
    .replace(/[\u{2190}-\u{21FF}\u{2300}-\u{23FF}\u{25A0}-\u{25FF}\u{2900}-\u{297F}]/gu, " ")
    .replace(/[\u{FE00}-\u{FE0F}]/gu, " ")
    .replace(/\u200D/gu, " ")
    .replace(/\u20E3/gu, " ")
    .replace(/[*_~#>|]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
}

interface UseSpeechOptions {
  onError?: (message: string) => void
}

/**
 * Reads assistant replies aloud. When a TTS model is configured, the text is
 * synthesized by PicoClaw (and played as audio); otherwise, or when the
 * request fails in "auto" mode, the browser's built-in speech synthesis is
 * used.
 */
export function useSpeech(options: UseSpeechOptions = {}) {
  const [isSpeaking, setIsSpeaking] = useState(false)
  const [isPreparing, setIsPreparing] = useState(false)
  const [isSupported, setIsSupported] = useState(false)
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const audioUrlRef = useRef<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const onErrorRef = useRef(options.onError)

  useEffect(() => {
    onErrorRef.current = options.onError
  }, [options.onError])

  const cleanupAudio = useCallback(() => {
    if (audioRef.current) {
      audioRef.current.pause()
      audioRef.current.src = ""
      audioRef.current = null
    }
    if (audioUrlRef.current) {
      URL.revokeObjectURL(audioUrlRef.current)
      audioUrlRef.current = null
    }
  }, [])

  const stop = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    cleanupAudio()
    stopSpeech()
    setIsSpeaking(false)
    setIsPreparing(false)
  }, [cleanupAudio])

  useEffect(() => {
    setIsSupported(speechSupported())
    return () => {
      stop()
    }
  }, [stop])

  const speakBrowser = useCallback(
    (plain: string, language: "tr" | "en" | ""): boolean => {
      const voice = pickSpeechVoice(plain, language)
      const utterance = speakWithVoice(
        plain,
        voice,
        {
          onEnd: () => setIsSpeaking(false),
          onError: () => setIsSpeaking(false),
        },
        language,
      )
      if (!utterance) {
        onErrorRef.current?.(
          "Read-aloud is not supported in this browser.",
        )
        return false
      }
      setIsSpeaking(true)
      return true
    },
    [],
  )

  const speak = useCallback(
    async (text: string) => {
      const plain = stripMarkdown(text)
      if (plain === "") return
      stop()

      const settings = readReadAloudSettings()
      const language = resolveReadAloudLanguage(plain, settings)

      if (settings.engine === "browser") {
        speakBrowser(plain, language)
        return
      }

      setIsPreparing(true)
      const controller = new AbortController()
      abortRef.current = controller
      try {
        const blob = await speakVoice(plain, language, controller.signal)
        if (controller.signal.aborted) return

        const url = URL.createObjectURL(blob)
        audioUrlRef.current = url
        const audio = new Audio(url)
        audioRef.current = audio
        audio.onended = () => {
          cleanupAudio()
          setIsSpeaking(false)
          setIsPreparing(false)
        }
        audio.onerror = () => {
          cleanupAudio()
          setIsSpeaking(false)
          setIsPreparing(false)
          onErrorRef.current?.("Could not play the synthesized audio.")
        }

        setIsPreparing(false)
        setIsSpeaking(true)
        try {
          await audio.play()
        } catch {
          cleanupAudio()
          setIsSpeaking(false)
          if (settings.engine === "auto") {
            speakBrowser(plain, language)
          } else {
            onErrorRef.current?.("Could not play the synthesized audio.")
          }
        }
      } catch (error) {
        if (controller.signal.aborted) return
        setIsPreparing(false)
        setIsSpeaking(false)
        if (settings.engine === "auto") {
          speakBrowser(plain, language)
          return
        }
        onErrorRef.current?.(
          error instanceof Error && error.message !== ""
            ? error.message
            : "Text-to-speech failed.",
        )
      }
    },
    [cleanupAudio, speakBrowser, stop],
  )

  const toggle = useCallback(
    (text: string) => {
      if (isSpeaking || isPreparing) {
        stop()
      } else {
        void speak(text)
      }
    },
    [isPreparing, isSpeaking, speak, stop],
  )

  return { isSupported, isSpeaking, isPreparing, speak, stop, toggle }
}
