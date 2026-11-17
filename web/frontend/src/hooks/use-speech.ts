import { useCallback, useEffect, useRef, useState } from "react"

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
  const utteranceRef = useRef<SpeechSynthesisUtterance | null>(null)

  useEffect(() => {
    const supported =
      typeof window !== "undefined" && "speechSynthesis" in window
    setIsSupported(supported)
    return () => {
      if (supported) {
        window.speechSynthesis.cancel()
      }
    }
  }, [])

  const pickVoice = useCallback((): SpeechSynthesisVoice | null => {
    const voices = window.speechSynthesis.getVoices()
    if (voices.length === 0) return null
    const lang = (
      document.documentElement.lang ||
      navigator.language ||
      "en"
    ).toLowerCase()
    const short = lang.split("-")[0]
    return (
      voices.find((voice) => voice.lang.toLowerCase() === lang) ??
      voices.find((voice) => voice.lang.toLowerCase().startsWith(short)) ??
      voices[0] ??
      null
    )
  }, [])

  const stop = useCallback(() => {
    if (typeof window === "undefined" || !("speechSynthesis" in window)) return
    window.speechSynthesis.cancel()
    utteranceRef.current = null
    setIsSpeaking(false)
  }, [])

  const speak = useCallback(
    (text: string) => {
      if (typeof window === "undefined" || !("speechSynthesis" in window)) return
      const plain = stripMarkdown(text)
      if (plain === "") return

      window.speechSynthesis.cancel()
      const utterance = new SpeechSynthesisUtterance(plain)
      const voice = pickVoice()
      if (voice) {
        utterance.voice = voice
        utterance.lang = voice.lang
      }
      utterance.onend = () => setIsSpeaking(false)
      utterance.onerror = () => setIsSpeaking(false)
      utteranceRef.current = utterance
      setIsSpeaking(true)
      window.speechSynthesis.speak(utterance)
    },
    [pickVoice],
  )

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
