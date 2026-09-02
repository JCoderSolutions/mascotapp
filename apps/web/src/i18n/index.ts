import { esMX, type TranslationKey } from './es-MX'

export type { TranslationKey }

/**
 * Translates a key, replacing {name} placeholders with the given params.
 *
 * A placeholder with no matching param is left as-is on purpose: printing
 * "undefined" in the UI hides the bug, while a visible {count} surfaces it.
 */
export function t(
  key: TranslationKey,
  params: Readonly<Record<string, string>> = {},
): string {
  return esMX[key].replace(/\{(\w+)\}/g, (placeholder, name: string) => {
    const value = params[name]
    return value ?? placeholder
  })
}
