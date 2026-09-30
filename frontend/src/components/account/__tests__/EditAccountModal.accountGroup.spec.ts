import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { AccountConfigGroup } from '@/api/admin/accountGroups'
import type { Account, AccountPlatform, AccountType } from '@/types'

const { accountEndpoints, updateGroup, showError } = vi.hoisted(() => ({
  accountEndpoints: {
    update: vi.fn(),
    checkMixedChannelRisk: vi.fn(),
    syncUpstreamModels: vi.fn(),
    getGrokMediaEligibility: vi.fn(),
    updateGrokMediaEligibility: vi.fn(),
    getOpenCodeGoUsage: vi.fn(),
    setOpenCodeGoUsageAutoRefresh: vi.fn(),
    refreshOpenCodeGoUsage: vi.fn(),
    getOllamaCloudUsage: vi.fn(),
    saveOllamaCloudUsageSession: vi.fn(),
    deleteOllamaCloudUsageSession: vi.fn(),
    setOllamaCloudUsageAutoRefresh: vi.fn(),
    refreshOllamaCloudUsage: vi.fn()
  },
  updateGroup: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn(), showInfo: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: accountEndpoints,
    accountGroups: { update: updateGroup },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: true, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn() }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'
import { createAccountGroupEditorAccount } from '../accountGroupEditor'

const BaseDialogStub = defineComponent({
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})
const SelectStub = defineComponent({
  props: ['modelValue', 'options', 'disabled'],
  emits: ['update:modelValue', 'change'],
  template: `<select :value="modelValue" :disabled="disabled" @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>`
})
const ToggleStub = defineComponent({
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: '<button type="button" :disabled="disabled" :aria-pressed="modelValue" @click="$emit(\'update:modelValue\', !modelValue)">toggle</button>'
})
const ModelSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: ['modelValue', 'platform', 'accountId'],
  template: '<div data-testid="model-selector" />'
})
const wrappers: VueWrapper[] = []

function group(platform: AccountPlatform = 'openai', type: AccountType = 'apikey', credentials: Record<string, unknown> = {}, extra: Record<string, unknown> = {}): AccountConfigGroup {
  return {
    id: 71, name: 'Shared account settings', group_id: 3, platform, type, account_ids: [11, 12],
    config: {
      notes: 'Shared notes', proxy_id: null, pool_id: null, concurrency: 4, priority: 9,
      rate_multiplier: 1.5, load_factor: null, status: 'active', schedulable: true,
      expires_at: null, auto_pause_on_expired: false, disable_auto_temp_unschedulable: false,
      credentials, extra
    },
    created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z'
  }
}
function mountGroup(value: AccountConfigGroup, accountOverrides: Partial<Account> = {}, realModelSelector = false) {
  const wrapper = mount(EditAccountModal, {
    props: {
      show: true,
      account: { ...createAccountGroupEditorAccount(value), ...accountOverrides },
      accountConfigGroup: value,
      proxies: [], pools: [], groups: []
    },
    global: { stubs: {
      BaseDialog: BaseDialogStub, Select: SelectStub, Toggle: ToggleStub,
      ModelWhitelistSelector: realModelSelector ? false : ModelSelectorStub,
      ModelIcon: true,
      Icon: true, ProxySelector: true, GroupSelector: true, QuotaLimitCard: true,
      ProxyAdBanner: true, ConfirmDialog: true
    } }
  })
  wrappers.push(wrapper)
  return wrapper
}
async function submit(wrapper: VueWrapper) {
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
  await flushPromises()
}
function expectNoAccountEndpoints() {
  for (const endpoint of Object.values(accountEndpoints)) expect(endpoint).not.toHaveBeenCalled()
}

beforeEach(() => {
  vi.clearAllMocks()
  updateGroup.mockImplementation(async (id, input) => ({ ...group(), id, config: input.config }))
  accountEndpoints.checkMixedChannelRisk.mockResolvedValue({ has_risk: false })
})
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
})

describe('EditAccountModal account-group typed settings', () => {
  it('shows both saved OpenCode Zen identity mappings in the mapping tab and keeps them across tab switches', async () => {
    const modelMapping = {
      'big-pickle': 'big-pickle',
      'space-bunny-free': 'space-bunny-free'
    }
    const wrapper = mountGroup(group('opencode_go', 'apikey', {
      account_mode: 'zen',
      api_protocol: 'adaptive',
      model_mapping: modelMapping
    }), {}, true)
    await flushPromises()

    // Use the actual selector so this exercises visible model chips, not only
    // the persistence payload or a stub that ignores its modelValue.
    const visibleModels = () => wrapper.text() + wrapper.findAll('input').map(input => input.element.value).join(' ')
    expect(visibleModels()).toContain('big-pickle')
    expect(visibleModels()).toContain('space-bunny-free')
    const mappingTab = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelMapping')!
    const whitelistTab = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelWhitelist')!
    await mappingTab.trigger('click')

    const mappingInputValues = () => wrapper.findAll('input[placeholder="admin.accounts.requestModel"]').map(input => input.element.value)
    expect(mappingInputValues()).toEqual(['big-pickle', 'space-bunny-free'])
    await whitelistTab.trigger('click')
    expect(wrapper.text()).toContain('big-pickle')
    expect(wrapper.text()).toContain('space-bunny-free')
    await mappingTab.trigger('click')
    expect(mappingInputValues()).toEqual(['big-pickle', 'space-bunny-free'])
    await submit(wrapper)
    expect(updateGroup.mock.calls[0]![1].config.credentials.model_mapping).toEqual(modelMapping)
  })

  it('deletes an identity mapping without a hidden whitelist entry restoring it on save', async () => {
    const wrapper = mountGroup(group('opencode_go', 'apikey', {
      account_mode: 'zen',
      model_mapping: { 'big-pickle': 'big-pickle', 'space-bunny-free': 'space-bunny-free' }
    }), {}, true)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelWhitelist')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    const rowInput = wrapper.findAll('input[placeholder="admin.accounts.requestModel"]').find(input => input.element.value === 'big-pickle')!
    await wrapper.findAll('button').find(button => button.element.parentElement === rowInput.element.parentElement)!.trigger('click')
    await submit(wrapper)

    expect(updateGroup.mock.calls[0]![1].config.credentials.model_mapping).toEqual({ 'space-bunny-free': 'space-bunny-free' })
  })

  it('displays identity entries and free-model aliases together without hiding either subset', async () => {
    const modelMapping = {
      'big-pickle': 'big-pickle',
      'space-bunny-free': 'space-bunny-free',
      'ling-3.0-flash-fin': 'ling-3.0-flash-fin-free',
      'longcat-2.5-preview': 'longcat-2.5-preview-free',
      'mimo-v2.5': 'mimo-v2.5-free',
      'mimo-v2.6-flash': 'mimo-v2.6-flash-free',
      'muse-spark-1.3': 'muse-spark-1.3-contributor-free',
      'nemotron-3-ultra': 'nemotron-3-ultra-free',
      'nemotron-3.5-lightning': 'nemotron-3.5-lightning-free'
    }
    const wrapper = mountGroup(group('opencode_go', 'apikey', {
      account_mode: 'zen', model_mapping: modelMapping
    }))
    await flushPromises()

    expect(wrapper.findAll('input[placeholder="admin.accounts.requestModel"]').map(input => input.element.value)).toEqual(Object.keys(modelMapping))
    expect(wrapper.findAll('input[placeholder="admin.accounts.actualModel"]').map(input => input.element.value)).toEqual(Object.values(modelMapping))
    await submit(wrapper)
    expect(updateGroup.mock.calls[0]![1].config.credentials.model_mapping).toEqual(modelMapping)
  })

  it('keeps an edited identity mapping target through whitelist and mapping tab switches', async () => {
    const wrapper = mountGroup(group('opencode_go', 'apikey', {
      account_mode: 'zen',
      model_mapping: { 'big-pickle': 'big-pickle', 'space-bunny-free': 'space-bunny-free' }
    }), {}, true)
    await flushPromises()
    await wrapper.findAll('input[placeholder="admin.accounts.actualModel"]').find(input => input.element.value === 'big-pickle')!.setValue('upstream-big-pickle')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelWhitelist')!.trigger('click')
    expect(wrapper.text()).toContain('space-bunny-free')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelMapping')!.trigger('click')
    expect(wrapper.findAll('input[placeholder="admin.accounts.actualModel"]').map(input => input.element.value)).toContain('upstream-big-pickle')
    await submit(wrapper)

    expect(updateGroup.mock.calls[0]![1].config.credentials.model_mapping).toEqual({
      'big-pickle': 'upstream-big-pickle', 'space-bunny-free': 'space-bunny-free'
    })
  })

  it.each(['apikey', 'oauth'] as AccountType[])('shows and independently edits OpenAI %s compact mapping rows', async type => {
    const wrapper = mountGroup(group('openai', type, {
      model_mapping: { 'request-model': 'request-model' },
      compact_model_mapping: { 'compact-identity': 'compact-identity', 'compact-alias': 'old-target' }
    }))
    await flushPromises()
    const compactRows = wrapper.findAll('input[placeholder="admin.accounts.fromModel"]')
    expect(compactRows.map(input => input.element.value)).toEqual(['compact-identity', 'compact-alias'])
    const identityRow = compactRows.find(input => input.element.value === 'compact-identity')!
    await wrapper.findAll('button').find(button => button.element.parentElement === identityRow.element.parentElement)!.trigger('click')
    await wrapper.findAll('input[placeholder="admin.accounts.toModel"]').find(input => input.element.value === 'old-target')!.setValue('new-target')
    await submit(wrapper)

    expect(updateGroup.mock.calls[0]![1].config.credentials).toMatchObject({
      model_mapping: { 'request-model': 'request-model' },
      compact_model_mapping: { 'compact-alias': 'new-target' }
    })
  })

  it.each([
    ['openai', 'apikey'], ['openai', 'oauth'], ['openai', 'setup-token'],
    ['anthropic', 'apikey'], ['anthropic', 'oauth'], ['anthropic', 'setup-token'],
    ['anthropic', 'bedrock'], ['anthropic', 'service_account'],
    ['gemini', 'apikey'], ['gemini', 'oauth'], ['gemini', 'service_account'],
    ['grok', 'apikey'], ['grok', 'oauth'],
    ['antigravity', 'oauth'], ['antigravity', 'upstream'],
    ['kiro', 'apikey'], ['kiro', 'oauth'],
    ['kimi', 'apikey'], ['zhipu', 'apikey'], ['deepseek', 'apikey'], ['opencode_go', 'apikey']
  ] as [AccountPlatform, AccountType][])('preserves every existing model mapping when saving unchanged %s / %s group settings', async (platform, type) => {
    const modelMapping = {
      'gpt-5.2': 'gpt-5.2',
      'request-alias': 'upstream-model',
      'claude-*': 'claude-sonnet-4-6',
      '*': 'fallback-model'
    }
    const compactModelMapping = { 'gpt-5.2': 'gpt-5.2-compact', '*': 'compact-fallback' }
    const value = group(platform, type, {
      model_mapping: modelMapping,
      ...(platform === 'openai' ? { compact_model_mapping: compactModelMapping } : {}),
      ...(type === 'bedrock' ? { aws_region: 'us-east-1' } : {}),
      ...(type === 'service_account' ? { location: 'us-central1' } : {}),
      ...(type === 'upstream' ? { base_url: 'https://relay.example' } : {})
    })
    const wrapper = mountGroup(value)
    await flushPromises()
    await submit(wrapper)

    expect(showError).not.toHaveBeenCalled()
    expect(updateGroup).toHaveBeenCalledTimes(1)
    expect(updateGroup.mock.calls[0]![1].config.credentials.model_mapping).toEqual(modelMapping)
    if (platform === 'openai') {
      expect(updateGroup.mock.calls[0]![1].config.credentials.compact_model_mapping).toEqual(compactModelMapping)
    }
    expect(value.config.credentials!.model_mapping).toEqual(modelMapping)
    expectNoAccountEndpoints()
  })

  it.each([
    { 'allowed-model': 'allowed-model' },
    { 'request-alias': 'upstream-model' },
    { '*': 'fallback-model' }
  ])('preserves OpenAI OAuth mapping %j while changing an unrelated group setting', async modelMapping => {
    const wrapper = mountGroup(group('openai', 'oauth', { model_mapping: modelMapping }))
    await flushPromises()
    await wrapper.get('[data-testid="account-rate-multiplier"]').setValue(2.5)
    await submit(wrapper)

    expect(updateGroup.mock.calls[0]![1].config).toMatchObject({
      rate_multiplier: 2.5,
      credentials: { model_mapping: modelMapping }
    })
  })

  it.each(['apikey', 'oauth'] as AccountType[])('keeps hidden OpenAI %s mappings with passthrough enabled', async type => {
    const credentials = {
      model_mapping: { 'request-alias': 'upstream-model', '*': 'fallback-model' },
      compact_model_mapping: { '*': 'compact-model' }
    }
    const wrapper = mountGroup(group('openai', type, credentials, { openai_passthrough: true }))
    await flushPromises()
    await wrapper.get('[data-testid="account-rate-multiplier"]').setValue(2.5)
    await submit(wrapper)

    expect(updateGroup.mock.calls[0]![1].config.credentials).toMatchObject(credentials)
  })

  it('loads the newly selected group mappings even though all synthetic account IDs are zero', async () => {
    const first = group('openai', 'oauth', {
      model_mapping: { 'first-alias': 'first-upstream' },
      compact_model_mapping: { '*': 'first-compact' }
    })
    const second = {
      ...group('openai', 'oauth', {
        model_mapping: { 'second-alias': 'second-upstream' },
        compact_model_mapping: { '*': 'second-compact' }
      }),
      id: 72
    }
    const wrapper = mountGroup(first)
    await flushPromises()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({
      show: true,
      account: createAccountGroupEditorAccount(second),
      accountConfigGroup: second
    })
    await flushPromises()
    await submit(wrapper)

    expect(updateGroup).toHaveBeenCalledTimes(1)
    expect(updateGroup.mock.calls[0]![0]).toBe(72)
    expect(updateGroup.mock.calls[0]![1].config.credentials).toMatchObject(second.config.credentials!)
  })

  it('saves OpenAI protocol and image controls through the group endpoint without API credentials', async () => {
    const value = group('openai', 'apikey', {
      base_url: 'https://upstream.example/v1',
      model_mapping: { 'gpt-5.2': 'gpt-5.2' }
    })
    const wrapper = mountGroup(value)
    await flushPromises()

    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.find('[data-tour="edit-account-form-name"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'GroupSelector' }).exists()).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.modelWhitelist')!.trigger('click')
    expect(wrapper.getComponent(ModelSelectorStub).props('accountId')).toBeUndefined()
    await wrapper.get('[data-testid="openai-responses-mode-select"]').setValue('force_responses')
    await wrapper.get('[data-testid="openai-images-url-to-b64-json-toggle"]').trigger('click')
    await wrapper.get('[data-testid="edit-openai-ws-mode-select"]').setValue('passthrough')
    await wrapper.get('[data-testid="upstream-billing-auto-probe"]').trigger('click')
    await submit(wrapper)

    expect(showError).not.toHaveBeenCalled()
    expect(updateGroup).toHaveBeenCalledTimes(1)
    const [id, input] = updateGroup.mock.calls[0]!
    expect(id).toBe(71)
    expect(input.config).toMatchObject({
      concurrency: 4, priority: 9, rate_multiplier: 1.5,
      credentials: { base_url: 'https://upstream.example/v1' },
      extra: {
        openai_responses_mode: 'force_responses',
        images_url_to_b64_json: true,
        openai_apikey_responses_websockets_v2_mode: 'passthrough',
        upstream_billing_probe_enabled: true
      }
    })
    expect(input.config.credentials).not.toHaveProperty('api_key')
    expect(input.config).not.toHaveProperty('name')
    expect(input.config).not.toHaveProperty('group_ids')
    expect(wrapper.emitted('group-updated')).toHaveLength(1)
    expect(wrapper.emitted('updated')).toBeUndefined()
    expectNoAccountEndpoints()
  })

  it.each([
    ['openai', 'oauth', { model_mapping: { 'gpt-5.2': 'gpt-5.2' } }],
    ['openai', 'setup-token', {}],
    ['anthropic', 'apikey', { base_url: 'https://api.anthropic.com' }],
    ['anthropic', 'oauth', {}],
    ['anthropic', 'setup-token', {}],
    ['anthropic', 'bedrock', { aws_region: 'us-east-1', aws_force_global: 'true' }],
    ['anthropic', 'service_account', { location: 'us-east5' }],
    ['gemini', 'service_account', { location: 'us-central1' }],
    ['gemini', 'apikey', { base_url: 'https://generativelanguage.googleapis.com' }],
    ['grok', 'apikey', { base_url: 'https://api.x.ai/v1' }],
    ['grok', 'oauth', {}],
    ['antigravity', 'oauth', { model_mapping: { a: 'b' } }],
    ['antigravity', 'upstream', { base_url: 'https://relay.example' }],
    ['kiro', 'apikey', { model_mapping: { a: 'b' } }],
    ['kiro', 'oauth', { model_mapping: { a: 'b' } }],
    ['kimi', 'apikey', { account_mode: 'coding', api_protocol: 'anthropic' }],
    ['zhipu', 'apikey', { account_mode: 'coding', api_protocol: 'chat_completions' }],
    ['deepseek', 'apikey', { account_mode: 'payg', api_protocol: 'chat_completions' }],
    ['opencode_go', 'apikey', { account_mode: 'go', api_protocol: 'adaptive' }]
  ] as [AccountPlatform, AccountType, Record<string, unknown>][])('submits %s / %s shared settings with no credential validation or member calls', async (platform, type, credentials) => {
    const wrapper = mountGroup(group(platform, type, credentials))
    await flushPromises()
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="antigravity-project-id-input"]').exists()).toBe(false)
    await submit(wrapper)
    expect(showError).not.toHaveBeenCalled()
    expect(updateGroup).toHaveBeenCalledTimes(1)
    const config = updateGroup.mock.calls[0]![1].config
    for (const key of ['api_key', 'access_token', 'refresh_token', 'aws_access_key_id', 'aws_secret_access_key', 'aws_session_token', 'project_id', 'client_email', 'service_account_json', 'antigravity_project_id']) {
      expect(config.credentials).not.toHaveProperty(key)
    }
    expectNoAccountEndpoints()
  })

  it('preserves Anthropic OAuth DTO-backed controls when submitting unchanged shared settings', async () => {
    const extra = {
      window_cost_limit: 20, window_cost_sticky_reserve: 4,
      max_sessions: 5, session_idle_timeout_minutes: 8,
      base_rpm: 40, rpm_strategy: 'sticky_exempt', rpm_sticky_buffer: 6,
      user_msg_queue_mode: 'serialize',
      enable_tls_fingerprint: true, tls_fingerprint_profile_id: 10,
      session_id_masking_enabled: true,
      cache_ttl_override_enabled: true, cache_ttl_override_target: '1h',
      custom_base_url_enabled: true, custom_base_url: 'https://relay.example'
    }
    const wrapper = mountGroup(group('anthropic', 'oauth', {}, extra))
    await submit(wrapper)
    expect(showError).not.toHaveBeenCalled()
    expect(updateGroup.mock.calls[0]![1].config.extra).toMatchObject(extra)
    expectNoAccountEndpoints()
  })

  it('persists and clears Grok media eligibility in shared extra, never its member endpoint', async () => {
    const wrapper = mountGroup(group('grok', 'oauth', {}, { grok_media_eligible: false }))
    await flushPromises()
    await wrapper.get('[data-testid="grok-media-eligibility-mode"]').setValue('auto')
    await submit(wrapper)
    expect(updateGroup.mock.calls[0]![1].config.extra).not.toHaveProperty('grok_media_eligible')
    expectNoAccountEndpoints()
  })

  it('does not load or mutate member usage panels, even if member runtime data is supplied', async () => {
    const wrapper = mountGroup(group('opencode_go', 'apikey', { account_mode: 'go', api_protocol: 'adaptive' }), {
      opencode_go_usage: { eligible: true, auto_refresh_enabled: true },
      ollama_cloud_usage: { eligible: true, configured: true, encryption_key_configured: true, auto_refresh_enabled: true }
    } as Partial<Account>)
    await flushPromises()
    expect(wrapper.find('[data-testid="opencode-go-refresh"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ollama-cloud-session-save"]').exists()).toBe(false)
    expectNoAccountEndpoints()
    await submit(wrapper)
    expect(updateGroup).toHaveBeenCalledTimes(1)
    expectNoAccountEndpoints()
  })

  it('retains fixed quota schedules and percentage notification controls on unchanged API-key groups', async () => {
    const extra = {
      quota_limit: 250, quota_daily_limit: 40, quota_weekly_limit: 100,
      quota_daily_reset_mode: 'fixed', quota_daily_reset_hour: 8,
      quota_weekly_reset_mode: 'fixed', quota_weekly_reset_day: 3, quota_weekly_reset_hour: 10,
      quota_reset_timezone: 'Asia/Shanghai',
      quota_notify_daily_enabled: true, quota_notify_daily_threshold: 10, quota_notify_daily_threshold_type: 'percentage',
      quota_notify_weekly_enabled: true, quota_notify_weekly_threshold: 15, quota_notify_weekly_threshold_type: 'percentage',
      quota_notify_total_enabled: true, quota_notify_total_threshold: 25, quota_notify_total_threshold_type: 'percentage',
      upstream_billing_probe_enabled: true,
      upstream_request_id_header: 'X-Upstream-Request-Id'
    }
    const wrapper = mountGroup(group('anthropic', 'apikey', {}, extra))
    await submit(wrapper)
    expect(showError).not.toHaveBeenCalled()
    expect(updateGroup.mock.calls[0]![1].config.extra).toMatchObject(extra)
    expectNoAccountEndpoints()
  })

  it('edits subscription tier and scheduler controls without sharing subscription credentials', async () => {
    const wrapper = mountGroup(group('openai', 'oauth', { plan_type: 'plus' }))
    const planSelector = wrapper.findAll('select').find(select => select.find('option[value="prolite"]').exists())!
    expect(planSelector.element.value).toBe('plus')
    await planSelector.setValue('pro')
    await wrapper.get('[data-testid="edit-codex-fingerprint-mode-select"]').setValue('device')
    await wrapper.get('[data-testid="account-group-schedulable"]').trigger('click')
    await submit(wrapper)
    expect(showError).not.toHaveBeenCalled()
    expect(updateGroup.mock.calls[0]![1].config).toMatchObject({
      schedulable: false,
      credentials: { plan_type: 'pro' },
      extra: { codex_fingerprint_mode: 'device' }
    })
    expectNoAccountEndpoints()
  })

  it('retains the selected proxy pool and normalizes empty expiry and load factor', async () => {
    const value = group('openai', 'oauth')
    value.config.pool_id = 9
    const wrapper = mountGroup(value)
    await submit(wrapper)
    expect(updateGroup.mock.calls[0]![1].config).toMatchObject({ pool_id: 9, proxy_id: null, expires_at: null, load_factor: null })
    expectNoAccountEndpoints()
  })

  it('shows why independent upstream-rate synchronization is disabled while the shared rate stays editable', async () => {
    const wrapper = mountGroup(group())
    expect(wrapper.get('[data-testid="upstream-billing-rate-sync"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.accountGroups.rateSyncManagedHint')
    await wrapper.get('[data-testid="account-rate-multiplier"]').setValue(2.5)
    await submit(wrapper)
    expect(updateGroup.mock.calls[0]![1].config.rate_multiplier).toBe(2.5)
    expectNoAccountEndpoints()
  })

  it('keeps the form open when a group save fails and does not fall back to member saves', async () => {
    updateGroup.mockRejectedValueOnce(new Error('Shared configuration conflict'))
    const wrapper = mountGroup(group('openai', 'oauth'))
    await submit(wrapper)
    expect(showError).toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('updated')).toBeUndefined()
    expect(wrapper.emitted('group-updated')).toBeUndefined()
    expectNoAccountEndpoints()
  })
})
