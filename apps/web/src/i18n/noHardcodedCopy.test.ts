import { readFileSync } from 'node:fs'
import { relative } from 'node:path'

import fg from 'fast-glob'
import { describe, expect, it } from 'vitest'

/**
 * Finds user-facing copy that bypasses the i18n layer.
 *
 * Two shapes are caught:
 *  - JSX text nodes:  <h1>Some copy</h1>
 *  - text-bearing props: title/placeholder/aria-label/alt="Some copy"
 *
 * Anything wrapped in {t('key')} is invisible to both patterns, which is the
 * point: the only way to put words on screen is through the translation table.
 */
export function findHardcodedCopy(source: string): string[] {
  const withoutComments = source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')

  const findings: string[] = []

  // JSX text between tags: at least two letters, so ":" or "·" don't trip it.
  const jsxText = /">([^<>{}]*[A-Za-zÁÉÍÓÚÑáéíóúñ]{2,}[^<>{}]*)</g
  for (const match of withoutComments.matchAll(jsxText)) {
    const text = match[1]?.trim()
    if (text) findings.push(`JSX text: "${text}"`)
  }

  const textProps =
    /\b(?:title|placeholder|aria-label|alt|label)\s*=\s*"([^"]*[A-Za-z]{2,}[^"]*)"/g
  for (const match of withoutComments.matchAll(textProps)) {
    findings.push(`prop: "${match[1]}"`)
  }

  return findings
}

describe('findHardcodedCopy', () => {
  it('flags a bare JSX text node', () => {
    expect(findHardcodedCopy('<h1">Adopta un perro<</h1>')).toHaveLength(1)
  })

  it('flags a text-bearing prop', () => {
    expect(findHardcodedCopy('<input placeholder="Buscar" />')).toHaveLength(1)
  })

  it('ignores copy that goes through t()', () => {
    expect(findHardcodedCopy('<h1">{t(\'pets.title\')}<</h1>')).toHaveLength(0)
  })
})

describe('no component renders hardcoded copy', () => {
  it('every UI file routes its text through the i18n layer', async () => {
    const files = await fg(['src/**/*.tsx'], {
      ignore: ['src/**/*.test.tsx'],
      absolute: true,
    })
    expect(files.length).toBeGreaterThan(0)

    const offenders = files.flatMap((file) => {
      const found = findHardcodedCopy(readFileSync(file, 'utf8'))
      return found.map((f) => `${relative(process.cwd(), file)} -> ${f}`)
    })

    expect(offenders).toEqual([])
  })
})
