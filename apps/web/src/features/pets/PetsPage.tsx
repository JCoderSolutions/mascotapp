import { EmptyState } from './components/EmptyState'
import { t } from '@/i18n'

/**
 * Container for the pet catalogue. Data fetching lands here in Phase 06;
 * for now it renders the empty state so the route is real, not a stub.
 */
export function PetsPage() {
  return (
    <section className="flex flex-col gap-6">
      <h1 className="text-2xl font-bold">{t('pets.title')}</h1>
      <EmptyState />
    </section>
  )
}
