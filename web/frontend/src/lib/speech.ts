export const SPEECH_VOICE_KEY = "picoclaw.speech.voice"

const TURKISH_CHARS = /[çğıöşüÇĞİÖŞÜ]/

const TURKISH_WORDS = new Set([
  "bir",
  "bu",
  "ve",
  "için",
  "icin",
  "ile",
  "değil",
  "degil",
  "mi",
  "mı",
  "mu",
  "mü",
  "ne",
  "nedir",
  "neydi",
  "kim",
  "kimdi",
  "neden",
  "niye",
  "nerede",
  "neresi",
  "hangi",
  "kaç",
  "kac",
  "nasıl",
  "nasil",
  "nasilsin",
  "nasılsın",
  "merhaba",
  "selam",
  "evet",
  "hayır",
  "hayir",
  "tamam",
  "lütfen",
  "lutfen",
  "çok",
  "cok",
  "az",
  "daha",
  "en",
  "gibi",
  "ama",
  "fakat",
  "ancak",
  "veya",
  "ya",
  "şey",
  "sey",
  "ben",
  "sen",
  "biz",
  "siz",
  "onlar",
  "bana",
  "sana",
  "onu",
  "bunu",
  "şunu",
  "sunu",
  "var",
  "yok",
  "olan",
  "olarak",
  "sonra",
  "önce",
  "once",
  "şimdi",
  "simdi",
  "zaman",
  "bugün",
  "bugun",
  "yarın",
  "yarin",
  "sence",
  "bence",
  "göre",
  "gore",
  "kadar",
  "üzere",
  "uzere",
  "çünkü",
  "cunku",
  "hep",
  "her",
  "hiç",
  "hic",
  "söyle",
  "soyle",
  "anlat",
  "cevap",
  "soru",
  "sor",
  "istiyorum",
  "bilmiyorum",
  "anlamıyorum",
  "anlamiyorum",
])

const ENGLISH_WORDS = new Set([
  "the",
  "and",
  "of",
  "is",
  "are",
  "you",
  "this",
  "that",
  "with",
  "for",
  "not",
  "have",
  "has",
  "was",
  "were",
  "what",
  "who",
  "when",
  "where",
  "why",
  "how",
  "hello",
  "thanks",
  "please",
  "from",
  "they",
  "will",
  "would",
  "can",
  "could",
  "about",
  "tell",
  "say",
  "think",
  "know",
  "want",
])

/**
 * Detects the language used by read-aloud. It is a heuristic: Turkish-specific
 * letters are a strong signal, otherwise common words decide. Very short or
 * ambiguous text returns "" so the caller can fall back to the user choice or
 * the browser language.
 */
export function detectSpeechLanguage(text: string): "tr" | "en" | "" {
  const trimmed = text.trim()
  if (trimmed === "") return ""
  if (TURKISH_CHARS.test(trimmed)) return "tr"

  const words = trimmed
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s]/gu, " ")
    .split(/\s+/)
    .filter(Boolean)
  if (words.length === 0) return ""

  let turkish = 0
  let english = 0
  for (const word of words) {
    if (TURKISH_WORDS.has(word)) turkish += 1
    if (ENGLISH_WORDS.has(word)) english += 1
  }

  if (turkish >= 2 && turkish > english) return "tr"
  if (english >= 2 && english > turkish) return "en"
  // A single strong word is enough for short greetings.
  if (words.length <= 3) {
    if (turkish > english) return "tr"
    if (english > turkish) return "en"
  }
  return ""
}

export function speechLanguageTag(text: string): string {
  switch (detectSpeechLanguage(text)) {
    case "tr":
      return "tr-TR"
    case "en":
      return "en-US"
    default:
      return ""
  }
}

function findVoiceForLanguage(
  voices: SpeechSynthesisVoice[],
  language: "tr" | "en",
): SpeechSynthesisVoice | null {
  const prefix = `${language}-`
  return (
    voices.find((voice) => voice.lang.toLowerCase() === language) ??
    voices.find((voice) => voice.lang.toLowerCase().startsWith(prefix)) ??
    null
  )
}

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
 * Picks the voice for read-aloud. Order: explicit user choice, the language of
 * the text itself, the browser language (the user's own locale, not the UI
 * language), then the first available voice.
 */
export function pickSpeechVoice(text = ""): SpeechSynthesisVoice | null {
  const voices = getSpeechVoices()
  if (voices.length === 0) return null

  const preferred = readSpeechVoicePreference()
  if (preferred !== "") {
    const chosen = voices.find((voice) => voice.voiceURI === preferred)
    if (chosen) return chosen
  }

  const textLanguage = detectSpeechLanguage(text)
  if (textLanguage !== "") {
    // If the matching voice is not installed, return null so speakWithVoice
    // can still pass the detected language tag; falling through to the browser
    // language would read Turkish text with an English voice.
    return findVoiceForLanguage(voices, textLanguage)
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
  } else {
    // Even without an installed voice, tell the engine which language to use.
    // Otherwise the browser silently falls back to its own default (often
    // English) and reads Turkish text with English pronunciation.
    const detected = speechLanguageTag(text)
    if (detected !== "") {
      utterance.lang = detected
    } else if (navigator.language) {
      utterance.lang = navigator.language
    }
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
