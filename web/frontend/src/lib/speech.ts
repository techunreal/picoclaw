export const SPEECH_VOICE_KEY = "picoclaw.speech.voice"

const TURKISH_CHARS = /[çğıöşüÇĞİÖŞÜ]/

export function speechSupported(): boolean {
  return typeof window !== "undefined" && "speechSynthesis" in window
}

export function getSpeechVoices(): SpeechSynthesisVoice[] {
  if (!speechSupported()) return []
  return window.speechSynthesis.getVoices()
}

export function readSpeechVoicePreference(): string {
  if (typeof window === "undefined") return ""
  try {
    return window.localStorage.getItem(SPEECH_VOICE_KEY) ?? ""
  } catch {
    return ""
  }
}

export function writeSpeechVoicePreference(voiceURI: string) {
  if (typeof window === "undefined") return
  try {
    if (voiceURI === "") {
      window.localStorage.removeItem(SPEECH_VOICE_KEY)
    } else {
      window.localStorage.setItem(SPEECH_VOICE_KEY, voiceURI)
    }
  } catch {
    // storage may be unavailable in private mode
  }
}

/**
 * Picks the voice for read-aloud. Order: explicit user choice, the browser
 * language (the user's own locale, not the UI language), then a Turkish voice
 * when the text looks Turkish, then the first available voice.
 */
export function pickSpeechVoice(text = ""): SpeechSynthesisVoice | null {
  const voices = getSpeechVoices()
  if (voices.length === 0) return null

  const preferred = readSpeechVoicePreference()
  if (preferred !== "") {
    const chosen = voices.find((voice) => voice.voiceURI === preferred)
    if (chosen) return chosen
  }

  const candidates = [navigator.language, document.documentElement.lang]
    .filter((value): value is string => Boolean(value))
    .map((value) => value.toLowerCase())

  for (const lang of candidates) {
    const short = lang.split("-")[0]
    const match =
      voices.find((voice) => voice.lang.toLowerCase() === lang) ??
      voices.find((voice) =>
        voice.lang.toLowerCase().startsWith(`${short}-`),
      ) ??
      voices.find((voice) => voice.lang.toLowerCase() === short)
    if (match) return match
  }

  if (TURKISH_CHARS.test(text)) {
    const turkish = voices.find((voice) =>
      voice.lang.toLowerCase().startsWith("tr"),
    )
    if (turkish) return turkish
  }

  return voices[0] ?? null
}

export function speakWithVoice(
  text: string,
  voice: SpeechSynthesisVoice | null,
  handlers?: {
    onEnd?: () => void
    onError?: () => void
  },
): SpeechSynthesisUtterance | null {
  if (!speechSupported()) return null
  const utterance = new SpeechSynthesisUtterance(text)
  if (voice) {
    utterance.voice = voice
    utterance.lang = voice.lang
  }
  utterance.onend = () => handlers?.onEnd?.()
  utterance.onerror = () => handlers?.onError?.()
  window.speechSynthesis.cancel()
  window.speechSynthesis.speak(utterance)
  return utterance
}

export function stopSpeech() {
  if (!speechSupported()) return
  window.speechSynthesis.cancel()
}
