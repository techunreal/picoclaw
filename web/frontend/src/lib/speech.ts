export const SPEECH_VOICE_KEY = "picoclaw.speech.voice"
export const READ_ALOUD_SETTINGS_KEY = "picoclaw.speech.readAloud"

export type ReadAloudEngine = "auto" | "tts" | "browser"
export type ReadAloudLanguage = "auto" | "tr" | "en"

export interface ReadAloudSettings {
  engine: ReadAloudEngine
  autoLanguage: boolean
  language: ReadAloudLanguage
  speed: number
}

export const DEFAULT_READ_ALOUD_SETTINGS: ReadAloudSettings = {
  engine: "auto",
  autoLanguage: true,
  language: "auto",
  speed: 1,
}

function clampSpeed(value: unknown): number {
  const numeric = typeof value === "number" ? value : Number(value)
  if (!Number.isFinite(numeric)) return 1
  return Math.min(2, Math.max(0.5, numeric))
}

/**
 * Splits read-aloud text into sentence-sized chunks. The first chunk can be
 * synthesized and played while later chunks are still being prepared.
 */
export function splitSpeechText(text: string, maxChars = 800): string[] {
  const normalized = text.replace(/\s+/g, " ").trim()
  if (normalized === "") return []
  const sentences = normalized.match(
    /[^.!?…]+[.!?…]+["'”’)\]]*|[^.!?…]+$/g,
  ) ?? [normalized]
  const chunks: string[] = []
  let current = ""

  const push = (piece: string) => {
    if (piece === "") return
    if (current === "") {
      current = piece
      return
    }
    if (current.length + 1 + piece.length <= maxChars) {
      current += " " + piece
      return
    }
    chunks.push(current)
    current = piece
  }

  const splitLong = (sentence: string) => {
    const words = sentence.split(" ")
    let part = ""
    for (const word of words) {
      if (word.length > maxChars) {
        if (part !== "") {
          chunks.push(part)
          part = ""
        }
        for (let index = 0; index < word.length; index += maxChars) {
          chunks.push(word.slice(index, index + maxChars))
        }
        continue
      }
      if (part === "") {
        part = word
      } else if ((part + " " + word).length <= maxChars) {
        part += " " + word
      } else {
        chunks.push(part)
        part = word
      }
    }
    if (part !== "") chunks.push(part)
  }

  for (const sentence of sentences) {
    const trimmed = sentence.trim()
    if (trimmed === "") continue
    if (trimmed.length <= maxChars) {
      push(trimmed)
    } else {
      if (current !== "") {
        chunks.push(current)
        current = ""
      }
      splitLong(trimmed)
    }
  }
  if (current !== "") chunks.push(current)
  return chunks.filter((chunk) => chunk.trim() !== "")
}

export function readReadAloudSettings(): ReadAloudSettings {
  if (typeof window === "undefined") return DEFAULT_READ_ALOUD_SETTINGS
  try {
    const raw = window.localStorage.getItem(READ_ALOUD_SETTINGS_KEY)
    if (raw === null) return DEFAULT_READ_ALOUD_SETTINGS
    const parsed = JSON.parse(raw) as Partial<ReadAloudSettings>
    const engine: ReadAloudEngine =
      parsed.engine === "tts" || parsed.engine === "browser"
        ? parsed.engine
        : "auto"
    const language: ReadAloudLanguage =
      parsed.language === "tr" || parsed.language === "en"
        ? parsed.language
        : "auto"
    return {
      engine,
      autoLanguage: parsed.autoLanguage !== false,
      language,
      speed: clampSpeed(parsed.speed ?? 1),
    }
  } catch {
    return DEFAULT_READ_ALOUD_SETTINGS
  }
}

export function writeReadAloudSettings(settings: ReadAloudSettings) {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(
      READ_ALOUD_SETTINGS_KEY,
      JSON.stringify(settings),
    )
  } catch {
    // storage may be unavailable in private mode
  }
}

/**
 * Resolves the language for read-aloud. A manual language always wins; when
 * automatic detection is enabled and no manual language is set, the text itself
 * decides. Empty means "no strong signal".
 */
export function resolveReadAloudLanguage(
  text: string,
  settings: ReadAloudSettings,
): "tr" | "en" | "" {
  if (settings.language !== "auto") return settings.language
  if (!settings.autoLanguage) return ""
  return detectSpeechLanguage(text)
}

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
export function pickSpeechVoice(
  text = "",
  languageOverride: "tr" | "en" | "" = "",
): SpeechSynthesisVoice | null {
  const voices = getSpeechVoices()
  if (voices.length === 0) return null

  if (languageOverride !== "") {
    return findVoiceForLanguage(voices, languageOverride)
  }

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

  return voices[0] ?? null
}

export function speakWithVoice(
  text: string,
  voice: SpeechSynthesisVoice | null,
  handlers?: {
    onEnd?: () => void
    onError?: () => void
  },
  languageOverride: "tr" | "en" | "" = "",
  rate = 1,
): SpeechSynthesisUtterance | null {
  if (!speechSupported()) return null
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.rate = clampSpeed(rate)
  const overrideTag =
    languageOverride === "tr"
      ? "tr-TR"
      : languageOverride === "en"
        ? "en-US"
        : ""
  const voiceMatchesOverride =
    voice === null ||
    overrideTag === "" ||
    voice.lang.toLowerCase().startsWith(`${languageOverride}-`)

  if (voice && voiceMatchesOverride) {
    utterance.voice = voice
    utterance.lang = voice.lang
  } else {
    // Even without a matching installed voice, tell the engine which language
    // to use. Otherwise the browser silently falls back to its own default
    // (often English) and reads Turkish text with English pronunciation.
    const detected = overrideTag || speechLanguageTag(text)
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
