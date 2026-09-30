import { describe, expect, it, vi } from 'vitest'

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } })
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() })
}))

describe('account groups route', () => {
  it('registers an authenticated admin-only route with translated navigation metadata', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find(record => record.name === 'AdminAccountGroups')
    expect(route?.path).toBe('/admin/account-groups')
    expect(route?.meta).toMatchObject({
      requiresAuth: true,
      requiresAdmin: true,
      titleKey: 'admin.accountGroups.title',
      descriptionKey: 'admin.accountGroups.description'
    })
    expect(route?.components?.default).toBeTypeOf('function')
  })

  it('resolves account edit links while preserving the target group query', async () => {
    const { default: router } = await import('@/router')
    const route = router.resolve({ path: '/admin/account-groups', query: { edit: 12 } })
    expect(route.name).toBe('AdminAccountGroups')
    expect(route.query.edit).toBe('12')
    expect(route.href).toBe('/admin/account-groups?edit=12')
  })
})
