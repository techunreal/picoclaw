import { useCallback, useEffect, useState } from "react"

import {
  pickSpeechVoice,
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

/**
 * Reads assistant replies with the browser's built-in speech synthesis.
 * No API key, model or server is required; voices come from the OS/browser.
 */
export function useSpeech() {
  const [isSpeaking, setIsSpeaking] = useState(false)
  const [isSupported, setIsSupported] = useState(false)

  useEffect(() => {
    setIsSupported(speechSupported())
    return () => {
      stopSpeech()
    }
  }, [])

  const stop = useCallback(() => {
    stopSpeech()
    setIsSpeaking(false)
  }, [])

  const speak = useCallback((text: string) => {
    const plain = stripMarkdown(text)
    if (plain === "") return
    const voice = pickSpeechVoice(plain)
    const utterance = speakWithVoice(plain, voice, {
      onEnd: () => setIsSpeaking(false),
      onError: () => setIsSpeaking(false),
    })
    if (utterance) {
      setIsSpeaking(true)
    }
  }, [])

  const toggle = useCallback(
    (text: string) => {
      if (isSpeaking) {
        stop()
      } else {
        speak(text)
      }
    },
    [isSpeaking, speak, stop],
  )

  return { isSupported, isSpeaking, speak, stop, toggle }
}
