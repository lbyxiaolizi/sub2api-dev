import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import type { AccountConfigGroup } from '@/api/admin/accountGroups'
import { buildAccountGroupEditorConfig, createAccountGroupEditorAccount } from '../accountGroupEditor'

function group(overrides: Partial<AccountConfigGroup> = {}): AccountConfigGroup {
  return {
    id: 41,
    name: 'Shared settings',
    group_id: 7,
    platform: 'anthropic',
    type: 'oauth',
    account_ids: [101, 102],
    config: {
      concurrency: 5,
      priority: 2,
      rate_multiplier: 1.2,
      status: 'active',
      schedulable: false,
      credentials: { model_mapping: { request: 'upstream' } },
      extra: {}
    },
    created_at: '2026-09-28T00:00:00Z',
    updated_at: '2026-09-28T00:00:00Z',
    ...overrides
  }
}

describe('account group typed editor model', () => {
  it('uses only the group snapshot, never member identity or credentials', () => {
    const source = group()
    source.config.credentials = {
      base_url: 'https://upstream.example.test',
      api_key: 'individual-api-key',
      access_token: 'individual-token',
      refresh_token: 'individual-refresh',
      project_id: 'individual-project',
      plan_type: 'plus',
      future_identity: 'individual',
      model_mapping: { request: 'upstream' }
    }
    source.config.extra = { quota_limit: 25, quota_used: 9, codex_fingerprint_seed: 'seed' }
    const account = createAccountGroupEditorAccount(source)
    expect(account.id).toBe(0)
    expect(source.account_ids).not.toContain(account.id)
    expect(account.account_config_group_id).toBe(41)
    expect(account.group_ids).toEqual([7])
    expect(account.schedulable).toBe(false)
    expect(account.credentials_status).toBeUndefined()
    expect(account.credentials).toEqual({
      base_url: 'https://upstream.example.test',
      plan_type: 'plus',
      model_mapping: { request: 'upstream' }
    })
    expect(account.extra).toEqual({ quota_limit: 25 })
    ;(account.credentials!.model_mapping as Record<string, string>).request = 'changed'
    expect(source.config.credentials!.model_mapping).toEqual({ request: 'upstream' })
  })

  it('projects Anthropic limits and session settings expected by the account editor DTO', () => {
    const source = group()
    source.config.extra = {
      window_cost_limit: 10,
      window_cost_sticky_reserve: 3,
      max_sessions: 6,
      session_idle_timeout_minutes: 7,
      base_rpm: 15,
      rpm_strategy: 'sticky_exempt',
      rpm_sticky_buffer: 5,
      user_msg_queue_mode: 'serialize',
      enable_tls_fingerprint: true,
      tls_fingerprint_profile_id: 4,
      session_id_masking_enabled: true,
      cache_ttl_override_enabled: true,
      cache_ttl_override_target: '1h',
      custom_base_url_enabled: true,
      custom_base_url: 'https://relay.example.test'
    }
    const account = createAccountGroupEditorAccount(source)
    expect(account).toMatchObject(source.config.extra)
    expect(account.extra).toEqual(source.config.extra)
  })
})

describe('account group typed editor payload', () => {
  it('preserves untouched top-level settings but replaces operational maps for clearing switches', () => {
    const current = group().config
    current.pool_id = 23
    current.credentials = { model_mapping: { request: 'old' }, pool_mode: true }
    current.extra = { grok_media_eligible: true, quota_limit: 50 }
    const payload = buildAccountGroupEditorConfig(current, {
      name: 'must-not-rename-the-group',
      group_ids: [999],
      account_ids: [999],
      credentials: {},
      extra: {},
      priority: 8
    })
    expect(payload).toEqual({
      concurrency: 5,
      priority: 8,
      rate_multiplier: 1.2,
      status: 'active',
      schedulable: false,
      pool_id: 23,
      proxy_id: null,
      credentials: {},
      extra: {}
    })
    expect(current.extra).toEqual({ grok_media_eligible: true, quota_limit: 50 })
  })

  it('maps billing probe settings and supports quota notification threshold types', () => {
    const result = buildAccountGroupEditorConfig(group().config, {
      upstream_billing_probe_enabled: true,
      upstream_billing_rate_sync_enabled: true,
      credentials: { api_key: 'secret', location: 'us-central1', client_email: 'identity' },
      extra: {
        quota_notify_daily_threshold_type: 'balance',
        quota_notify_weekly_threshold_type: 'percentage',
        quota_notify_total_threshold_type: 'balance',
        quota_daily_used: 42,
        grok_media_eligible: false
      }
    })
    expect(result.credentials).toEqual({ location: 'us-central1' })
    expect(result.extra).toEqual({
      upstream_billing_probe_enabled: true,
      quota_notify_daily_threshold_type: 'balance',
      quota_notify_weekly_threshold_type: 'percentage',
      quota_notify_total_threshold_type: 'balance',
      grok_media_eligible: false
    })
    expect(result).not.toHaveProperty('upstream_billing_probe_enabled')
    expect(result).not.toHaveProperty('upstream_billing_rate_sync_enabled')
  })

  it('normalizes individual clearing sentinels and proxy pool exclusivity', () => {
    const cleared = buildAccountGroupEditorConfig(group().config, {
      pool_id: 0, proxy_id: 0, load_factor: 0, expires_at: 0
    })
    expect(cleared).toMatchObject({ pool_id: null, proxy_id: null, load_factor: null, expires_at: null })
    const pooled = buildAccountGroupEditorConfig(group().config, { pool_id: 4, proxy_id: 12 })
    expect(pooled).toMatchObject({ pool_id: 4, proxy_id: null })
  })
})

// This is a cross-boundary contract: a new server-managed setting must never
// disappear merely because a group was opened and saved through the typed UI.
describe('account group shared settings contract', () => {
  it('matches the backend credential and extra allowlists', () => {
    const backend = readFileSync(
      resolve(process.cwd(), '../backend/internal/service/account_config_group.go'),
      'utf8'
    )
    const frontend = readFileSync(resolve(process.cwd(), 'src/components/account/accountGroupEditor.ts'), 'utf8')
    for (const [serverName, clientName] of [
      ['accountConfigGroupCredentialKeys', 'CREDENTIAL_SETTING_KEYS'],
      ['accountConfigGroupExtraKeys', 'EXTRA_SETTING_KEYS']
    ]) {
      const serverBody = backend.split(`var ${serverName} = map[string]bool{`)[1]?.split('\n}')[0]
      const clientBody = frontend.split(`const ${clientName} = new Set([`)[1]?.split('])')[0]
      expect(serverBody).toBeDefined()
      expect(clientBody).toBeDefined()
      const serverKeys = Array.from(serverBody!.matchAll(/"([a-z0-9_]+)":\s*true/g), m => m[1]).sort()
      const clientKeys = Array.from(clientBody!.matchAll(/'([a-z0-9_]+)'/g), m => m[1]).sort()
      expect(clientKeys, `${clientName} must stay aligned with ${serverName}`).toEqual(serverKeys)
    }
  })
})
