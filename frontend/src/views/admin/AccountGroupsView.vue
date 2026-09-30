<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.accountGroups.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accountGroups.description') }}</p>
          </div>
          <div class="flex gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="loadData">{{ t('common.refresh') }}</button>
            <button class="btn btn-primary" :disabled="loading" data-testid="create-account-group" @click="openCreate">
              <Icon name="plus" size="sm" />{{ t('admin.accountGroups.create') }}
            </button>
          </div>
        </div>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="accountGroups" :loading="loading">
          <template #empty>
            <EmptyState :title="t('admin.accountGroups.empty')" :description="t('admin.accountGroups.emptyHint')" />
          </template>
          <template #cell-group_id="{ value }">{{ parentGroups.find(group => group.id === value)?.name ?? `#${value}` }}</template>
          <template #cell-platform="{ row }">{{ row.platform }} / {{ row.type }}</template>
          <template #cell-account_ids="{ value }">{{ value?.length ?? 0 }}</template>
          <template #cell-actions="{ row }">
            <div class="flex flex-wrap gap-2">
              <button v-if="canCreateAccount(row)" class="btn btn-primary btn-sm" :disabled="!!openingMemberGroup || creatingMember" :data-testid="`create-account-group-member-${row.id}`" @click="openCreateAccount(row)"><Icon name="plus" size="sm" />{{ t('admin.accountGroups.createAccount') }}</button>
              <button class="btn btn-primary btn-sm" :data-testid="`account-group-settings-${row.id}`" @click="openSettings(row)">{{ t('admin.accountGroups.accountSettings') }}</button>
              <button class="btn btn-secondary btn-sm" :data-testid="`edit-account-group-${row.id}`" @click="openEdit(row)">{{ t('common.edit') }}</button>
              <button class="btn btn-danger btn-sm" @click="groupToDelete = row">{{ t('admin.accountGroups.dissolve') }}</button>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <BaseDialog :show="showForm" :title="editingGroup ? t('admin.accountGroups.edit') : t('admin.accountGroups.create')" width="extra-wide" :close-on-escape="!pendingMappingOverwrite" @close="closeForm">
      <form id="account-group-form" class="space-y-5" @submit.prevent="save">
        <p class="rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-900/20 dark:text-blue-200">{{ t('admin.accountGroups.syncHint') }}</p>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label for="account-group-name" class="input-label">{{ t('common.name') }}</label>
            <input id="account-group-name" v-model.trim="form.name" class="input" required maxlength="100" :disabled="saving" />
          </div>
          <div>
            <label for="account-group-parent" class="input-label">{{ t('admin.accountGroups.parentGroup') }}</label>
            <select id="account-group-parent" v-model.number="form.group_id" class="input" :disabled="!!editingGroup || saving" required @change="changeParentGroup">
              <option :value="0" disabled>{{ t('admin.accountGroups.selectParentGroup') }}</option>
              <option v-for="group in parentGroups" :key="group.id" :value="group.id">{{ group.name }}</option>
            </select>
          </div>
        </div>

        <fieldset class="space-y-3">
          <legend class="input-label">{{ t('admin.accountGroups.members') }} ({{ form.account_ids.length }})</legend>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountGroups.membersHint') }}</p>
          <input v-model="memberSearch" type="search" class="input" :aria-label="t('admin.accountGroups.searchAccounts')" :placeholder="t('admin.accountGroups.searchAccounts')" />
          <div v-if="accountsLoading" class="py-4 text-sm text-gray-500">{{ t('common.loading') }}</div>
          <div v-else-if="candidatesError" class="text-sm text-red-600" role="alert">{{ candidatesError }}</div>
          <div v-else class="max-h-56 space-y-1 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-dark-600">
            <label v-for="account in visibleCandidates" :key="account.id" class="flex items-center gap-3 rounded p-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-700" :class="{ 'opacity-50': !canSelect(account) }">
              <input type="checkbox" :checked="form.account_ids.includes(account.id)" :disabled="!canSelect(account) || saving" :data-testid="`member-${account.id}`" @change="toggleMember(account.id)" />
              <span class="min-w-0 flex-1 truncate">{{ account.name }} <span class="text-gray-400">#{{ account.id }}</span></span>
              <span class="text-xs text-gray-500">{{ memberReason(account) || `${account.platform} / ${account.type}` }}</span>
            </label>
            <p v-if="visibleCandidates.length === 0" class="p-3 text-sm text-gray-500">{{ t('admin.accountGroups.noAccounts') }}</p>
          </div>
          <div v-if="!editingGroup && form.account_ids.length">
            <label for="account-group-source" class="input-label">{{ t('admin.accountGroups.sourceAccount') }}</label>
            <select id="account-group-source" v-model.number="form.source_account_id" class="input" required :disabled="saving">
              <option v-for="account in selectedCandidates" :key="account.id" :value="account.id">{{ account.name }} (#{{ account.id }})</option>
            </select>
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.accountGroups.sourceHint') }}</p>
          </div>
        </fieldset>

        <p v-if="editingGroup" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accountGroups.membersOnlyHint') }}</p>
        <p v-if="formError" class="text-sm text-red-600" role="alert">{{ formError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" :disabled="saving" @click="closeForm">{{ t('common.cancel') }}</button>
          <button form="account-group-form" type="submit" class="btn btn-primary" :disabled="saving || accountsLoading || !!candidatesError || !form.group_id || !form.account_ids.length" data-testid="save-account-group">
            {{ saving ? t('common.saving') : t('admin.accountGroups.saveAndSync') }}
          </button>
        </div>
      </template>
    </BaseDialog>
    <BaseDialog
      :show="!!newMemberGroup"
      :title="t('admin.accountGroups.createAccountTitle', { name: newMemberGroup?.name ?? '' })"
      :close-on-escape="!creatingMember"
      :show-close-button="!creatingMember"
      @close="closeCreateAccount"
    >
      <form id="account-group-member-form" class="space-y-5" @submit.prevent="createAccount">
        <p class="rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-900/20 dark:text-blue-200">{{ t('admin.accountGroups.createAccountHint') }}</p>
        <div>
          <label for="account-group-member-base-url" class="input-label">{{ t('admin.accountGroups.apiAddress') }}</label>
          <input id="account-group-member-base-url" v-model.trim="newMemberForm.base_url" type="url" class="input" :disabled="creatingMember" :placeholder="t('admin.accountGroups.apiAddressPlaceholder')" autocomplete="off" spellcheck="false" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountGroups.apiAddressHint') }}</p>
        </div>
        <div>
          <label for="account-group-member-api-key" class="input-label">API Key</label>
          <input id="account-group-member-api-key" v-model.trim="newMemberForm.api_key" type="password" class="input" required :disabled="creatingMember" autocomplete="new-password" spellcheck="false" />
        </div>
        <p v-if="newMemberError" class="text-sm text-red-600" role="alert">{{ newMemberError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" :disabled="creatingMember" data-testid="cancel-account-group-member" @click="closeCreateAccount">{{ t('common.cancel') }}</button>
          <button form="account-group-member-form" type="submit" class="btn btn-primary" :disabled="creatingMember || !newMemberForm.api_key.trim()" data-testid="save-account-group-member">{{ creatingMember ? t('common.saving') : t('admin.accountGroups.createAccount') }}</button>
        </div>
      </template>
    </BaseDialog>
    <EditAccountModal
      v-if="settingsGroup"
      :show="true"
      :account="settingsAccount"
      :account-config-group="settingsGroup"
      :proxies="proxies"
      :pools="proxyPools"
      :groups="parentGroups"
      @close="settingsGroup = null"
      @group-updated="onSettingsUpdated"
    />
    <ConfirmDialog
      :show="!!pendingMappingOverwrite"
      :title="t('admin.accountGroups.mappingConflictTitle')"
      :message="mappingConflictMessage"
      :confirm-text="t('admin.accountGroups.mappingConflictConfirm')"
      :cancel-text="t('admin.accountGroups.mappingConflictReview')"
      :danger="true"
      @confirm="confirmMappingOverwrite"
      @cancel="clearMappingOverwrite"
    >
      <p v-if="mappingConflictDetails" class="text-sm text-amber-700 dark:text-amber-300" data-testid="account-group-mapping-conflict-details">{{ mappingConflictDetails }}</p>
    </ConfirmDialog>
    <ConfirmDialog :show="!!groupToDelete" :title="t('admin.accountGroups.dissolve')" :message="t('admin.accountGroups.dissolveConfirm', { name: groupToDelete?.name ?? '' })" :confirm-text="t('admin.accountGroups.dissolve')" @confirm="dissolve" @cancel="groupToDelete = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AccountConfigGroup, CreateAccountGroupRequest, UpdateAccountGroupRequest } from '@/api/admin/accountGroups'
import type { AccountListItem, AdminGroup, Proxy, ProxyPoolWithStats } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorCode, extractApiErrorMessage, extractApiErrorMetadata } from '@/utils/apiError'
import EditAccountModal from '@/components/account/EditAccountModal.vue'
import { createAccountGroupEditorAccount } from '@/components/account/accountGroupEditor'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const accountGroups = ref<AccountConfigGroup[]>([])
const parentGroups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const proxyPools = ref<ProxyPoolWithStats[]>([])
const loading = ref(false)
const saving = ref(false)
const showForm = ref(false)
const editingGroup = ref<AccountConfigGroup | null>(null)
const groupToDelete = ref<AccountConfigGroup | null>(null)
const candidates = ref<AccountListItem[]>([])
const accountsLoading = ref(false)
const candidatesError = ref('')
const memberSearch = ref('')
const formError = ref('')
const form = reactive({ name: '', group_id: 0, account_ids: [] as number[], source_account_id: 0 })
const settingsGroup = ref<AccountConfigGroup | null>(null)
const settingsAccount = computed(() => settingsGroup.value ? createAccountGroupEditorAccount(settingsGroup.value) : null)
const openingMemberGroup = ref<number | null>(null)
const newMemberGroup = ref<AccountConfigGroup | null>(null)
const creatingMember = ref(false)
const newMemberError = ref('')
const newMemberForm = reactive({ base_url: '', api_key: '' })
const newMemberDefaultBaseURL = ref('')
// Keep retry metadata in memory only; never persist or log the credential payload.
let newMemberOperation: { payload: string; key: string } | null = null
type PendingGroupSave =
  | { kind: 'create'; input: CreateAccountGroupRequest }
  | { kind: 'update'; id: number; input: UpdateAccountGroupRequest }
const pendingMappingOverwrite = ref<PendingGroupSave | null>(null)
const mappingConflictMetadata = ref<Record<string, unknown>>({})
const mappingConflictMessage = computed(() => {
  const pending = pendingMappingOverwrite.value
  if (!pending) return ''
  if (pending.kind === 'create') {
    const sourceID = pending.input.source_account_id
    const source = candidates.value.find(account => account.id === sourceID)
    return t('admin.accountGroups.mappingConflictCreate', { source: source ? `${source.name} (#${source.id})` : `#${sourceID}` })
  }
  return t('admin.accountGroups.mappingConflictJoin', { name: pending.input.name ?? editingGroup.value?.name ?? '' })
})
const mappingConflictDetails = computed(() => {
  const metadata = mappingConflictMetadata.value
  const accountID = Number(metadata.account_id)
  if (!Number.isInteger(accountID) || accountID <= 0) return ''
  const account = candidates.value.find(item => item.id === accountID)
  return t('admin.accountGroups.mappingConflictDetails', {
    account: account ? `${account.name} (#${accountID})` : `#${accountID}`,
    field: t(metadata.field === 'compact_model_mapping'
      ? 'admin.accountGroups.mappingConflictCompactField'
      : 'admin.accountGroups.mappingConflictModelField'),
    removed: String(metadata.removed_entries ?? '?'),
    changed: String(metadata.changed_entries ?? '?')
  })
})
let candidatesRequest = 0

const columns = computed(() => [
  { key: 'name', label: t('common.name') },
  { key: 'group_id', label: t('admin.accountGroups.parentGroup') },
  { key: 'platform', label: t('admin.accountGroups.platformType') },
  { key: 'account_ids', label: t('admin.accountGroups.members') },
  { key: 'actions', label: t('common.actions') }
])
const occupiedAccounts = computed(() => new Map(accountGroups.value.filter(group => group.id !== editingGroup.value?.id).flatMap(group => group.account_ids.map(id => [id, group.name] as const))))
const selectedCandidates = computed(() => candidates.value.filter(account => form.account_ids.includes(account.id)))
const memberType = computed(() => editingGroup.value ?? selectedCandidates.value[0])
const visibleCandidates = computed(() => {
  const search = memberSearch.value.trim().toLowerCase()
  return candidates.value.filter(account => !search || account.name.toLowerCase().includes(search) || String(account.id).includes(search))
})

function memberReason(account: AccountListItem): string {
  const owner = occupiedAccounts.value.get(account.id)
  if (owner) return t('admin.accountGroups.alreadyMember', { name: owner })
  if (account.parent_account_id) return t('admin.accountGroups.shadowAccount')
  if (memberType.value && (memberType.value.platform !== account.platform || memberType.value.type !== account.type)) return t('admin.accountGroups.incompatible')
  return ''
}
function canSelect(account: AccountListItem): boolean { return !memberReason(account) }
function toggleMember(id: number) {
  form.account_ids = form.account_ids.includes(id) ? form.account_ids.filter(value => value !== id) : [...form.account_ids, id]
  if (!form.account_ids.includes(form.source_account_id)) form.source_account_id = form.account_ids[0] ?? 0
}

async function loadData() {
  loading.value = true
  try {
    const [groups, parents, proxyList, pools] = await Promise.all([
      adminAPI.accountGroups.list(), adminAPI.groups.getAllIncludingInactive(), adminAPI.proxies.getAll(), adminAPI.proxyPools.list()
    ])
    accountGroups.value = groups
    parentGroups.value = parents
    proxies.value = proxyList
    proxyPools.value = pools
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.accountGroups.loadFailed')))
  } finally { loading.value = false }
}

async function loadCandidates() {
  const request = ++candidatesRequest
  candidates.value = []
  candidatesError.value = ''
  if (!form.group_id) return
  const groupID = form.group_id
  accountsLoading.value = true
  try {
    const all: AccountListItem[] = []
    let page = 1
    while (true) {
      const result = await adminAPI.accounts.list(page, 100, { group: String(groupID), lite: '1' })
      if (request !== candidatesRequest) return
      all.push(...result.items)
      if (!result.items.length || page >= result.pages) break
      page++
    }
    candidates.value = all
  } catch (error) {
    if (request === candidatesRequest) candidatesError.value = extractApiErrorMessage(error, t('admin.accountGroups.loadAccountsFailed'))
  } finally { if (request === candidatesRequest) accountsLoading.value = false }
}
function changeParentGroup() {
  form.account_ids = []
  form.source_account_id = 0
  void loadCandidates()
}
function openCreate() {
  clearMappingOverwrite()
  editingGroup.value = null
  Object.assign(form, { name: '', group_id: 0, account_ids: [], source_account_id: 0 })
  memberSearch.value = ''
  formError.value = ''
  candidates.value = []
  candidatesError.value = ''
  showForm.value = true
}
async function openEdit(group: AccountConfigGroup) {
  clearMappingOverwrite()
  formError.value = ''
  try {
    const fresh = await adminAPI.accountGroups.getById(group.id)
    editingGroup.value = fresh
    Object.assign(form, { name: fresh.name, group_id: fresh.group_id, account_ids: [...fresh.account_ids], source_account_id: 0 })
    memberSearch.value = ''
    showForm.value = true
    await loadCandidates()
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.accountGroups.loadFailed'))) }
}
function closeForm() { if (!saving.value) { clearMappingOverwrite(); showForm.value = false; candidatesRequest++; accountsLoading.value = false } }
async function openSettings(group: AccountConfigGroup) {
  try {
    settingsGroup.value = await adminAPI.accountGroups.getById(group.id)
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.accountGroups.loadFailed'))) }
}
async function onSettingsUpdated() {
  settingsGroup.value = null
  await loadData()
}
function canCreateAccount(group: AccountConfigGroup): boolean {
  return group.type === 'apikey' || group.type === 'upstream'
}
async function openCreateAccount(group: AccountConfigGroup) {
  if (openingMemberGroup.value || creatingMember.value || !canCreateAccount(group)) return
  openingMemberGroup.value = group.id
  try {
    // Fetch the latest shared settings without reading existing members' secrets.
    const fresh = await adminAPI.accountGroups.getById(group.id)
    if (!canCreateAccount(fresh)) return
    const baseURL = fresh.default_base_url ?? fresh.config.credentials?.base_url
    newMemberDefaultBaseURL.value = typeof baseURL === 'string' ? baseURL.trim() : ''
    Object.assign(newMemberForm, { base_url: newMemberDefaultBaseURL.value, api_key: '' })
    newMemberOperation = null
    newMemberError.value = ''
    newMemberGroup.value = fresh
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.accountGroups.loadFailed')))
  } finally { openingMemberGroup.value = null }
}
function clearNewMemberForm() {
  newMemberGroup.value = null
  Object.assign(newMemberForm, { base_url: '', api_key: '' })
  newMemberDefaultBaseURL.value = ''
  newMemberError.value = ''
  newMemberOperation = null
}
function closeCreateAccount() {
  if (!creatingMember.value) clearNewMemberForm()
}
async function createAccount() {
  if (!newMemberGroup.value || creatingMember.value) return
  newMemberError.value = ''
  const apiKey = newMemberForm.api_key.trim()
  if (!apiKey) { newMemberError.value = t('admin.accountGroups.apiKeyRequired'); return }
  const baseURL = newMemberForm.base_url.trim()
  if (baseURL) {
    try {
      const url = new URL(baseURL)
      if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password || url.search || url.hash) throw new Error('invalid URL')
    } catch {
      newMemberError.value = t('admin.accountGroups.apiAddressInvalid')
      return
    }
  }
  creatingMember.value = true
  try {
    const input = {
      api_key: apiKey,
      // Unchanged values follow the current group default, not a frozen override.
      ...(baseURL && baseURL !== newMemberDefaultBaseURL.value ? { base_url: baseURL } : {})
    }
    const payload = JSON.stringify({ group_id: newMemberGroup.value.id, ...input })
    if (newMemberOperation?.payload !== payload) {
      const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
      newMemberOperation = { payload, key: `account-group-member-${newMemberGroup.value.id}-${requestID}` }
    }
    await adminAPI.accountGroups.createAccount(newMemberGroup.value.id, input, newMemberOperation.key)
    clearNewMemberForm()
    appStore.showSuccess(t('admin.accountGroups.accountCreated'))
    await loadData()
  } catch (error) {
    newMemberError.value = extractApiErrorMessage(error, t('admin.accountGroups.createAccountFailed'))
  } finally { creatingMember.value = false }
}
function clearMappingOverwrite() {
  pendingMappingOverwrite.value = null
  mappingConflictMetadata.value = {}
}
function currentSaveRequest(): PendingGroupSave {
  return editingGroup.value
    ? { kind: 'update', id: editingGroup.value.id, input: { name: form.name, account_ids: [...form.account_ids] } }
    : { kind: 'create', input: { name: form.name, group_id: form.group_id, account_ids: [...form.account_ids], source_account_id: form.source_account_id } }
}
async function save() {
  if (saving.value || pendingMappingOverwrite.value) return
  formError.value = ''
  if (!form.name || !form.group_id || !form.account_ids.length) { formError.value = t('admin.accountGroups.required'); return }
  await submitGroupSave(currentSaveRequest())
}
async function confirmMappingOverwrite() {
  if (saving.value || !pendingMappingOverwrite.value) return
  // Consume the confirmation before sending. It never sticks to the form or a later save.
  const pending = pendingMappingOverwrite.value
  clearMappingOverwrite()
  await submitGroupSave(pending, true)
}
async function submitGroupSave(request: PendingGroupSave, confirmed = false) {
  saving.value = true
  formError.value = ''
  try {
    let created: AccountConfigGroup | undefined
    const confirmation = confirmed ? { confirm_model_mapping_overwrite: true } : {}
    if (request.kind === 'update') {
      await adminAPI.accountGroups.update(request.id, { ...request.input, ...confirmation })
    } else {
      created = await adminAPI.accountGroups.create({ ...request.input, ...confirmation })
    }
    showForm.value = false
    clearMappingOverwrite()
    appStore.showSuccess(t('admin.accountGroups.saved'))
    await loadData()
    if (created) settingsGroup.value = created
  } catch (error) {
    if (extractApiErrorCode(error) === 'ACCOUNT_CONFIG_GROUP_MAPPING_CONFLICT' &&
      JSON.stringify(request) === JSON.stringify(currentSaveRequest())) {
      pendingMappingOverwrite.value = request
      mappingConflictMetadata.value = extractApiErrorMetadata(error) ?? {}
    } else {
      formError.value = extractApiErrorMessage(error, t('admin.accountGroups.saveFailed'))
    }
  } finally { saving.value = false }
}
// Any change invalidates a warning for an older source/member selection.
watch(form, clearMappingOverwrite, { deep: true, flush: 'sync' })
async function dissolve() {
  if (!groupToDelete.value || saving.value) return
  saving.value = true
  try {
    await adminAPI.accountGroups.remove(groupToDelete.value.id)
    groupToDelete.value = null
    appStore.showSuccess(t('admin.accountGroups.dissolved'))
    await loadData()
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('admin.accountGroups.saveFailed'))) }
  finally { saving.value = false }
}
function openQueryGroup() {
  const id = Number(route.query.edit)
  const group = accountGroups.value.find(item => item.id === id)
  if (group && !showForm.value && !settingsGroup.value) void openSettings(group)
}
watch(() => route.query.edit, openQueryGroup)
onMounted(async () => { await loadData(); openQueryGroup() })
</script>
