import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { EmptyState } from './EmptyState'

describe('EmptyState', () => {
  it('renders the translated title and body', () => {
    render(<EmptyState />)

    expect(
      screen.getByRole('heading', {
        name: 'Todavía no hay animales publicados',
      }),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Cuando un refugio publique un animal, aparecerá acá.'),
    ).toBeInTheDocument()
  })

  it('announces itself to assistive technology as a status region', () => {
    render(<EmptyState />)

    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('renders a retry action only when a handler is provided', () => {
    const { rerender } = render(<EmptyState />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()

    rerender(<EmptyState onRetry={() => {}} />)
    expect(
      screen.getByRole('button', { name: 'Reintentar' }),
    ).toBeInTheDocument()
  })
})
