import type { Account } from '@/types'
import type { AccountConfigGroup, AccountGroupConfig } from '@/api/admin/accountGroups'

// Keep aligned with backend/internal/service/account_config_group.go. Only
// operational settings cross this boundary; unknown identity/runtime keys do not.
const CREDENTIAL_SETTING_KEYS = new Set([
  'plan_type',
  'base_url',
  'api_base_urls',
  'api_protocol',
  'model_mapping',
  'compact_model_mapping',
  'header_overrides',
  'custom_error_codes',
  'custom_error_codes_enabled',
  'pool_mode',
  'pool_mode_retry_count',
  'pool_mode_retry_status_codes',
  'temp_unschedulable_enabled',
  'temp_unschedulable_rules',
  'openai_capabilities',
  'protocol_rules',
  'header_override_enabled',
  'intercept_warmup_requests',
  'account_scheduling_threshold',
  'account_mode',
  'model_whitelist',
  'aws_region',
  'aws_force_global',
  'location',
  'vertex_location',
])

const EXTRA_SETTING_KEYS = new Set([
  'upstream_billing_probe_enabled',
  'allow_overages',
  'anthropic_apikey_auth_scheme',
  'anthropic_passthrough',
  'auto_pause_5h_disabled',
  'auto_pause_5h_threshold',
  'auto_pause_7d_disabled',
  'auto_pause_7d_threshold',
  'auto_reset_credit_5h_threshold',
  'auto_reset_credit_7d_threshold',
  'auto_reset_credit_enabled',
  'base_rpm',
  'cache_ttl_override_enabled',
  'cache_ttl_override_target',
  'codex_cli_only',
  'codex_cli_only_allow_app_server',
  'codex_cli_only_allowed_clients',
  'codex_fingerprint_mode',
  'codex_image_generation_bridge',
  'codex_image_generation_bridge_enabled',
  'codex_image_generation_explicit_tool_policy',
  'custom_base_url',
  'custom_base_url_enabled',
  'enable_tls_fingerprint',
  'grok_client_tool_cache_enabled',
  'grok_media_eligible',
  'images_url_to_b64_json',
  'kiro_credit_unit_price_usd',
  'max_sessions',
  'mixed_scheduling',
  'openai_apikey_responses_websockets_v2_enabled',
  'openai_apikey_responses_websockets_v2_mode',
  'openai_compact_mode',
  'openai_long_context_billing_enabled',
  'openai_oauth_passthrough',
  'openai_oauth_responses_websockets_v2_enabled',
  'openai_oauth_responses_websockets_v2_mode',
  'openai_passthrough',
  'openai_responses_flatten_namespaces',
  'openai_responses_mode',
  'openai_ws_allow_store_recovery',
  'openai_ws_enabled',
  'openai_ws_force_http',
  'quota_daily_limit',
  'quota_daily_reset_hour',
  'quota_daily_reset_mode',
  'quota_limit',
  'quota_notify_daily_enabled',
  'quota_notify_daily_threshold',
  'quota_notify_daily_threshold_type',
  'quota_notify_total_enabled',
  'quota_notify_total_threshold',
  'quota_notify_total_threshold_type',
  'quota_notify_weekly_enabled',
  'quota_notify_weekly_threshold',
  'quota_notify_weekly_threshold_type',
  'quota_reset_timezone',
  'quota_weekly_limit',
  'quota_weekly_reset_day',
  'quota_weekly_reset_hour',
  'quota_weekly_reset_mode',
  'responses_websockets_v2_enabled',
  'rpm_sticky_buffer',
  'rpm_strategy',
  'session_id_masking_enabled',
  'session_idle_timeout_minutes',
  'tls_fingerprint_profile_id',
  'upstream_request_id_header',
  'user_msg_queue_enabled',
  'user_msg_queue_mode',
  'web_search_emulation',
  'window_cost_limit',
  'window_cost_sticky_reserve',
])

const CONFIG_KEYS = [
  'notes', 'proxy_id', 'pool_id', 'concurrency', 'priority', 'rate_multiplier',
  'load_factor', 'status', 'schedulable', 'expires_at', 'auto_pause_on_expired',
  'disable_auto_temp_unschedulable'
] as const

const FLATTENED_EXTRA_KEYS = [
  'window_cost_limit', 'window_cost_sticky_reserve', 'max_sessions',
  'session_idle_timeout_minutes', 'base_rpm', 'rpm_strategy', 'rpm_sticky_buffer',
  'user_msg_queue_mode', 'enable_tls_fingerprint', 'tls_fingerprint_profile_id',
  'session_id_masking_enabled', 'cache_ttl_override_enabled', 'cache_ttl_override_target',
  'custom_base_url_enabled', 'custom_base_url', 'quota_limit', 'quota_daily_limit',
  'quota_weekly_limit', 'quota_daily_reset_mode', 'quota_daily_reset_hour',
  'quota_weekly_reset_mode', 'quota_weekly_reset_day', 'quota_weekly_reset_hour',
  'quota_reset_timezone'
] as const

function settingsOnly(value: unknown, keys: Set<string>): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return JSON.parse(JSON.stringify(Object.fromEntries(
    Object.entries(value).filter(([key]) => keys.has(key))
  ))) as Record<string, unknown>
}

/** Build the existing typed editor's model without reading any member account. */
export function createAccountGroupEditorAccount(group: AccountConfigGroup): Account {
  const config = buildAccountGroupEditorConfig(group.config, {})
  const extra = config.extra ?? {}
  const flattened = Object.fromEntries(
    FLATTENED_EXTRA_KEYS.filter(key => key in extra).map(key => [key, extra[key]])
  )
  return {
    // A group is not an account: never use a member ID for editor-side probes.
    id: 0,
    name: group.name,
    platform: group.platform,
    type: group.type,
    notes: config.notes ?? '',
    credentials: config.credentials,
    extra,
    proxy_id: config.proxy_id ?? null,
    pool_id: config.pool_id ?? null,
    concurrency: config.concurrency ?? 1,
    priority: config.priority ?? 1,
    rate_multiplier: config.rate_multiplier ?? 1,
    load_factor: config.load_factor ?? null,
    status: config.status ?? 'active',
    schedulable: config.schedulable ?? true,
    expires_at: config.expires_at ?? null,
    auto_pause_on_expired: config.auto_pause_on_expired ?? true,
    disable_auto_temp_unschedulable: config.disable_auto_temp_unschedulable ?? false,
    group_ids: [group.group_id],
    account_config_group_id: group.id,
    account_config_group_name: group.name,
    created_at: group.created_at,
    updated_at: group.updated_at,
    error_message: null,
    last_used_at: null,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...flattened
  }
}

/** Convert a normal account form payload to a complete group settings snapshot. */
export function buildAccountGroupEditorConfig(
  current: AccountGroupConfig,
  payload: Record<string, unknown>
): AccountGroupConfig {
  const combined = { ...current, ...payload }
  const config = Object.fromEntries(
    CONFIG_KEYS.filter(key => combined[key] !== undefined).map(key => [key, combined[key]])
  ) as AccountGroupConfig
  config.credentials = settingsOnly(combined.credentials, CREDENTIAL_SETTING_KEYS)
  config.extra = settingsOnly(combined.extra, EXTRA_SETTING_KEYS)
  // Individual update APIs expose this setting at the top level; groups store it in extra.
  if (typeof payload.upstream_billing_probe_enabled === 'boolean') {
    config.extra.upstream_billing_probe_enabled = payload.upstream_billing_probe_enabled
  }
  // The account editor uses 0 as the individual-update clearing sentinel.
  for (const key of ['proxy_id', 'pool_id', 'load_factor', 'expires_at'] as const) {
    if (config[key] != null && config[key]! <= 0) config[key] = null
  }
  if (config.pool_id != null) config.proxy_id = null
  return config
}
