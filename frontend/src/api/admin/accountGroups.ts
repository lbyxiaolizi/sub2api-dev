/**
 * Centrally managed account configuration groups.
 *
 * These are distinct from API-key groups: every member must belong to the
 * selected API-key group, share its platform/type and belong to only one
 * configuration group. Authentication credentials remain account-specific.
 */
import { apiClient } from '../client'
import type { Account, AccountPlatform, AccountType, UpdateAccountRequest } from '@/types'

/**
 * Shared operational settings, not account identity or group bindings.
 * `credentials` and `extra` accept operational settings only; the server
 * rejects authentication secrets, identity fields and runtime state.
 */
export type AccountGroupConfig = Pick<
  UpdateAccountRequest,
  | 'notes'
  | 'proxy_id'
  | 'pool_id'
  | 'concurrency'
  | 'priority'
  | 'rate_multiplier'
  | 'load_factor'
  | 'status'
  | 'schedulable'
  | 'expires_at'
  | 'auto_pause_on_expired'
  | 'disable_auto_temp_unschedulable'
  | 'credentials'
  | 'extra'
>

export interface AccountConfigGroup {
  id: number
  name: string
  group_id: number
  platform: AccountPlatform
  type: AccountType
  account_ids: number[]
  config: AccountGroupConfig
  /** Effective default endpoint, including adaptive/provider-specific defaults. */
  default_base_url?: string
  created_at: string
  updated_at: string
}

export interface CreateAccountGroupRequest {
  name: string
  group_id: number
  account_ids: number[]
  /** Member whose operational settings initialize the group. */
  source_account_id?: number
  /** Explicit, one-request confirmation to replace differing member model mappings. */
  confirm_model_mapping_overwrite?: boolean
}

export interface UpdateAccountGroupRequest {
  name?: string
  /** Replaces the complete membership list when supplied. */
  account_ids?: number[]
  /** Changes are applied to every member together. */
  config?: AccountGroupConfig
  /** Explicit, one-request confirmation to replace newly joined members’ model mappings. */
  confirm_model_mapping_overwrite?: boolean
}

export interface CreateAccountGroupMemberRequest {
  /** Omit or leave blank to use the group's current API address. */
  base_url?: string
  /** This credential belongs only to the new member. */
  api_key: string
}

export async function list(): Promise<AccountConfigGroup[]> {
  const { data } = await apiClient.get<AccountConfigGroup[]>('/admin/account-groups')
  return data
}

export async function getById(id: number): Promise<AccountConfigGroup> {
  const { data } = await apiClient.get<AccountConfigGroup>(`/admin/account-groups/${id}`)
  return data
}

export async function create(input: CreateAccountGroupRequest): Promise<AccountConfigGroup> {
  const { data } = await apiClient.post<AccountConfigGroup>('/admin/account-groups', input)
  return data
}

export async function update(id: number, input: UpdateAccountGroupRequest): Promise<AccountConfigGroup> {
  const { data } = await apiClient.put<AccountConfigGroup>(`/admin/account-groups/${id}`, input)
  return data
}

/** Creates and joins a member atomically using the group's shared settings. */
export async function createAccount(id: number, input: CreateAccountGroupMemberRequest, idempotencyKey?: string): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/account-groups/${id}/accounts`, input,
    idempotencyKey ? { headers: { 'Idempotency-Key': idempotencyKey } } : undefined)
  return data
}

/** Dissolves the group without deleting members or reverting their settings. */
export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/account-groups/${id}`)
}

export default { list, getById, create, update, createAccount, remove }
