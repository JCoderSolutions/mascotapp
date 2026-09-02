import {
  createRootRoute,
  createRoute,
  createRouter,
  Link,
  Outlet,
} from '@tanstack/react-router'

import { PetsPage } from '@/features/pets/PetsPage'
import { t } from '@/i18n'

const rootRoute = createRootRoute({
  component: () => (
    <div className="min-h-dvh">
      <nav className="flex gap-4 border-b p-4">
        <Link to="/" className="font-semibold">
          {t('app.name')}
        </Link>
        <Link to="/pets">{t('nav.pets')}</Link>
      </nav>
      <main className="p-6">
        <Outlet />
      </main>
    </div>
  ),
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: () => <h1 className="text-2xl font-bold">{t('app.name')}</h1>,
})

const petsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pets',
  component: PetsPage,
})

const routeTree = rootRoute.addChildren([indexRoute, petsRoute])

export const router = createRouter({ routeTree })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
