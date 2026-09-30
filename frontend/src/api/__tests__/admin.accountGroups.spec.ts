import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), remove: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post, put, delete: remove } }))
import accountGroups from '@/api/admin/accountGroups'

describe('admin account groups API', () => {
  const group = { id: 9, name: 'Shared', group_id: 3, platform: 'openai', type: 'oauth', account_ids: [1, 2], config: { concurrency: 5 } }
  beforeEach(() => vi.resetAllMocks())

  it('lists groups and fetches a group without pagination or credentials hydration', async () => {
    get.mockResolvedValueOnce({ data: [group] }).mockResolvedValueOnce({ data: group })
    await expect(accountGroups.list()).resolves.toEqual([group])
    await expect(accountGroups.getById(9)).resolves.toEqual(group)
    expect(get).toHaveBeenNthCalledWith(1, '/admin/account-groups')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/account-groups/9')
  })

  it('creates from a member source without sending its authentication credentials', async () => {
    const input = { name: 'Shared', group_id: 3, account_ids: [1, 2], source_account_id: 2 }
    post.mockResolvedValueOnce({ data: group })
    await expect(accountGroups.create(input)).resolves.toEqual(group)
    expect(post).toHaveBeenCalledWith('/admin/account-groups', input)
  })

  it('updates membership and shared configuration including explicit clearing', async () => {
    const input = { account_ids: [1, 3], config: { concurrency: 0, rate_multiplier: 0, proxy_id: null, schedulable: false, extra: { auto_pause_5h_disabled: false } } }
    put.mockResolvedValueOnce({ data: group })
    await expect(accountGroups.update(9, input)).resolves.toEqual(group)
    expect(put).toHaveBeenCalledWith('/admin/account-groups/9', input)
  })

  it('creates a member with only its independent API key and optional API address', async () => {
    const account = { id: 12, name: 'Shared #12', account_config_group_id: 9 }
    post.mockResolvedValue({ data: account })
    await expect(accountGroups.createAccount(9, { api_key: 'new-key' })).resolves.toEqual(account)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/account-groups/9/accounts', { api_key: 'new-key' }, undefined)
    const custom = { api_key: 'another-key', base_url: 'https://member.example/v1' }
    await expect(accountGroups.createAccount(9, custom, 'member-operation')).resolves.toEqual(account)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/account-groups/9/accounts', custom, { headers: { 'Idempotency-Key': 'member-operation' } })
  })

  it('forwards explicit model mapping overwrite confirmation only on the requested operation', async () => {
    const createInput = { name: 'Shared', group_id: 3, account_ids: [1, 2], source_account_id: 2, confirm_model_mapping_overwrite: true }
    const updateInput = { account_ids: [1, 3], confirm_model_mapping_overwrite: true }
    post.mockResolvedValueOnce({ data: group })
    put.mockResolvedValue({ data: group })
    await accountGroups.create(createInput)
    await accountGroups.update(9, updateInput)
    await accountGroups.update(9, { account_ids: [1, 4] })
    expect(post).toHaveBeenCalledWith('/admin/account-groups', createInput)
    expect(put).toHaveBeenNthCalledWith(1, '/admin/account-groups/9', updateInput)
    expect(put).toHaveBeenNthCalledWith(2, '/admin/account-groups/9', { account_ids: [1, 4] })
  })

  it('dissolves a group and propagates server membership conflicts', async () => {
    remove.mockResolvedValueOnce({ data: {} })
    await expect(accountGroups.remove(9)).resolves.toBeUndefined()
    expect(remove).toHaveBeenCalledWith('/admin/account-groups/9')
    const conflict = { status: 409, message: 'Account already belongs to another group' }
    put.mockRejectedValueOnce(conflict)
    await expect(accountGroups.update(9, { account_ids: [2] })).rejects.toEqual(conflict)
  })
})
