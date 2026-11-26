import { useCallback, useEffect, useRef, useState } from "react"

import { speakVoice } from "@/api/voice"
import {
  pickSpeechVoice,
  readReadAloudSettings,
  resolveReadAloudLanguage,
  speakWithVoice,
  speechSupported,
  splitSpeechText,
  stopSpeech,
} from "@/lib/speech"

function stripMarkdown(text: string): string {
  return (
    text
      .replace(/```[\s\S]*?```/g, " ")
      .replace(/`([^`]*)`/g, "$1")
      .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
      .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
      // Emoji, dingbats, variation selectors and zero-width joiners: never read aloud.
      .replace(
        /\p{Extended_Pictographic}(\uFE0F|\u200D\p{Extended_Pictographic})*/gu,
        " ",
      )
      .replace(/[\u{1F000}-\u{1FAFF}]/gu, " ")
      .replace(/[\u{2600}-\u{27BF}]/gu, " ")
      .replace(/[\u{2B00}-\u{2BFF}]/gu, " ")
      .replace(
        /[\u{2190}-\u{21FF}\u{2300}-\u{23FF}\u{25A0}-\u{25FF}\u{2900}-\u{297F}]/gu,
        " ",
      )
      .replace(/[\u{FE00}-\u{FE0F}]/gu, " ")
      .replace(/\u200D/gu, " ")
      .replace(/\u20E3/gu, " ")
      .replace(/[*_~#>|]/g, " ")
      .replace(/\s+/g, " ")
      .trim()
  )
}

interface UseSpeechOptions {
  onError?: (message: string) => void
}

export interface SpeechProgress {
  index: number
  total: number
}

const SPEECH_CHUNK_CHARS = 800

/**
 * Reads assistant replies aloud. When a TTS model is configured, the text is
 * synthesized by PicoClaw in sentence-sized chunks and played in sequence;
 * otherwise, or when the first request fails in "auto" mode, the browser's
 * built-in speech synthesis is used.
 */
export function useSpeech(options: UseSpeechOptions = {}) {
  const [isSpeaking, setIsSpeaking] = useState(false)
  const [isPreparing, setIsPreparing] = useState(false)
  const [isSupported, setIsSupported] = useState(false)
  const [progress, setProgress] = useState<SpeechProgress | null>(null)
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const audioUrlRef = useRef<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const finishPlayRef = useRef<(() => void) | null>(null)
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
    finishPlayRef.current?.()
    finishPlayRef.current = null
    cleanupAudio()
    stopSpeech()
    setIsSpeaking(false)
    setIsPreparing(false)
    setProgress(null)
  }, [cleanupAudio])

  useEffect(() => {
    setIsSupported(speechSupported())
    return () => {
      stop()
    }
  }, [stop])

  const speakBrowser = useCallback(
    (plain: string, language: "tr" | "en" | "", speed: number): boolean => {
      const voice = pickSpeechVoice(plain, language)
      const utterance = speakWithVoice(
        plain,
        voice,
        {
          onEnd: () => setIsSpeaking(false),
          onError: () => setIsSpeaking(false),
        },
        language,
        speed,
      )
      if (!utterance) {
        onErrorRef.current?.("Read-aloud is not supported in this browser.")
        return false
      }
      setIsSpeaking(true)
      return true
    },
    [],
  )

  const playBlob = useCallback(async (blob: Blob): Promise<boolean> => {
    const url = URL.createObjectURL(blob)
    const audio = new Audio(url)
    audioRef.current = audio
    audioUrlRef.current = url
    return new Promise<boolean>((resolve) => {
      let settled = false
      const finish = (ok: boolean) => {
        if (settled) return
        settled = true
        finishPlayRef.current = null
        if (audioRef.current === audio) {
          audio.pause()
          audioRef.current = null
        }
        if (audioUrlRef.current === url) {
          URL.revokeObjectURL(url)
          audioUrlRef.current = null
        }
        resolve(ok)
      }
      finishPlayRef.current = () => finish(false)
      audio.onended = () => finish(true)
      audio.onerror = () => finish(false)
      audio.play().catch(() => finish(false))
    })
  }, [])

  const speak = useCallback(
    async (text: string) => {
      const plain = stripMarkdown(text)
      if (plain === "") return
      stop()

      const settings = readReadAloudSettings()
      const language = resolveReadAloudLanguage(plain, settings)
      const speed = settings.speed

      if (settings.engine === "browser") {
        speakBrowser(plain, language, speed)
        return
      }

      const chunks = splitSpeechText(plain, SPEECH_CHUNK_CHARS)
      if (chunks.length === 0) return

      setIsPreparing(true)
      setProgress({ index: 1, total: chunks.length })
      const controller = new AbortController()
      abortRef.current = controller

      let nextChunk: Promise<Blob> | null = speakVoice(
        chunks[0],
        language,
        controller.signal,
        speed,
      )

      for (let index = 0; index < chunks.length; index += 1) {
        try {
          const pending = nextChunk
          if (!pending) break
          const blob = await pending
          if (controller.signal.aborted) return
          nextChunk =
            index + 1 < chunks.length
              ? speakVoice(
                  chunks[index + 1],
                  language,
                  controller.signal,
                  speed,
                )
              : null

          setIsPreparing(false)
          setIsSpeaking(true)
          setProgress({ index: index + 1, total: chunks.length })
          const played = await playBlob(blob)
          if (controller.signal.aborted) return
          if (!played) {
            if (settings.engine === "auto" && index === 0) {
              speakBrowser(plain, language, speed)
            } else {
              onErrorRef.current?.("Could not play the synthesized audio.")
            }
            stop()
            return
          }
        } catch (error) {
          if (controller.signal.aborted) return
          stop()
          if (settings.engine === "auto" && index === 0) {
            speakBrowser(plain, language, speed)
            return
          }
          onErrorRef.current?.(
            error instanceof Error && error.message !== ""
              ? error.message
              : "Text-to-speech failed.",
          )
          return
        }
      }

      abortRef.current = null
      setIsSpeaking(false)
      setIsPreparing(false)
      setProgress(null)
    },
    [playBlob, speakBrowser, stop],
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

  return { isSupported, isSpeaking, isPreparing, progress, speak, stop, toggle }
}
