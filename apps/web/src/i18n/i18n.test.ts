import { describe, expect, it } from 'vitest'

import { t } from '@/i18n'

describe('t', () => {
  it('returns the translation for a known key', () => {
    expect(t('pets.emptyState.title')).toBe('Todavía no hay animales publicados')
  })

  it('interpolates named parameters', () => {
    expect(t('pets.count', { count: '3' })).toBe('3 animales disponibles')
  })

  it('leaves an unmatched placeholder untouched rather than printing undefined', () => {
    expect(t('pets.count', {})).toBe('{count} animales disponibles')
  })
})
