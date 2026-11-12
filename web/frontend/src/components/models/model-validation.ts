/**
 * Real-time model field validation utilities.
 * All checks are pure frontend, no network required.
 *
 * Messages use i18n keys with interpolation params — callers must
 * translate them via t(key, params).
 */
import type { ModelProviderOption } from "@/api/models"

import {
  findClosestProvider,
  getCanonicalProviderKey,
  getKnownProviderKeys,
} from "./provider-registry"

export type ValidationLevel = "error" | "warning" | "success"

export interface FieldValidation {
  level: ValidationLevel
  messageKey: string
  messageParams?: Record<string, string>
  fix?: string
}

/**
 * Validate a model identifier string with optional provider context.
 * Returns validation result with optional one-click fix suggestion.
 */
export function validateModelField(
  input: string,
  selectedProvider?: string,
  backendOptions?: ModelProviderOption[],
): FieldValidation {
  const trimmed = input.trim()
  if (!trimmed) return { level: "success", messageKey: "" }
  const knownProviderKeys = getKnownProviderKeys(backendOptions)

  // Hard errors
  if (/\s/.test(trimmed)) {
    return {
      level: "error",
      messageKey: "models.validation.whitespace",
      fix: trimmed.replace(/\s+/g, "/"),
    }
  }
  if (trimmed.startsWith("/")) {
    return {
      level: "error",
      messageKey: "models.validation.leadingSlash",
      fix: trimmed.replace(/^\/+/, ""),
    }
  }
  if (trimmed.includes("//")) {
    return {
      level: "error",
      messageKey: "models.validation.consecutiveSlash",
      fix: trimmed.replace(/\/+/g, "/"),
    }
  }

  // When a provider is explicitly selected, the field is a full model ID for
  // that provider. Slashes may be part of a case-sensitive upstream namespace
  // (for example "ACME/gpt-4o-mini"), so do not reinterpret the prefix as a
  // PicoClaw provider key.
  if (selectedProvider) {
    return {
      level: "success",
      messageKey: "models.validation.parsed",
      messageParams: { provider: selectedProvider, model: trimmed },
    }
  }

  const slashIdx = trimmed.indexOf("/")
  if (slashIdx === -1) {
    return {
      level: "warning",
      messageKey: "models.validation.defaultToOpenAI",
      fix: `openai/${trimmed}`,
    }
  }

  const provider = trimmed.slice(0, slashIdx)
  const model = trimmed.slice(slashIdx + 1)
  if (!model) {
    return { level: "error", messageKey: "models.validation.emptyModel" }
  }

  if (!knownProviderKeys.has(provider)) {
    // Check aliases. Only suggest a rewrite when the canonical form is a
    // provider we actually know; otherwise a case-sensitive upstream namespace
    // would be lowercased by the one-click fix.
    const alias = getCanonicalProviderKey(provider, backendOptions)
    if (alias && alias !== provider && knownProviderKeys.has(alias)) {
      return {
        level: "warning",
        messageKey: "models.validation.shouldUse",
        messageParams: { provider, alias },
        fix: `${alias}/${model}`,
      }
    }
    // Typo check
    const closest = findClosestProvider(provider, backendOptions)
    if (closest) {
      return {
        level: "warning",
        messageKey: "models.validation.didYouMean",
        messageParams: { closest },
        fix: `${closest}/${model}`,
      }
    }
    return {
      level: "warning",
      messageKey: "models.validation.unknownProvider",
      messageParams: { provider },
    }
  }

  return {
    level: "success",
    messageKey: "models.validation.parsed",
    messageParams: { provider, model },
  }
}
