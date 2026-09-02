/**
 * Base locale. Every user-facing string in the app lives here — components
 * must never hardcode copy. Adding a locale means adding a file with the
 * same keys; the Translations type below makes a missing key a type error.
 */
export const esMX = {
  'app.name': 'MascotApp',
  'nav.home': 'Inicio',
  'nav.pets': 'Animales',
  'pets.title': 'Animales en adopción',
  'pets.count': '{count} animales disponibles',
  'pets.emptyState.title': 'Todavía no hay animales publicados',
  'pets.emptyState.body':
    'Cuando un refugio publique un animal, aparecerá acá.',
  'common.retry': 'Reintentar',
} as const

export type TranslationKey = keyof typeof esMX
