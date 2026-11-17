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
