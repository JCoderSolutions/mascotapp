import { cn } from '@/lib/cn'
import { t } from '@/i18n'

type EmptyStateProps = {
  /** When provided, a retry action is offered. Omit it for a purely informational state. */
  onRetry?: () => void
  className?: string
}

/**
 * Presentational empty state for the pet catalogue. Takes props and renders —
 * no data fetching. The status role makes screen readers announce it when the
 * list resolves to nothing, instead of leaving the user in silence.
 */
export function EmptyState({ onRetry, className }: EmptyStateProps) {
  return (
    <div
      role="status"
      className={cn(
        'flex flex-col items-center gap-3 rounded-lg border border-dashed p-10 text-center',
        className,
      )}
    >
      <h2 className="text-lg font-semibold">{t('pets.emptyState.title')}</h2>
      <p className="text-sm opacity-80">{t('pets.emptyState.body')}</p>
      {onRetry ? (
        <button
          type="button"
          onClick={onRetry}
          className="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white hover:bg-brand-700"
        >
          {t('common.retry')}
        </button>
      ) : null}
    </div>
  )
}
