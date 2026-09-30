import Select from '@/components/common/Select.vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AccountGroupsView from '../AccountGroupsView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), get: vi.fn(), create: vi.fn(), update: vi.fn(), createAccount: vi.fn(), remove: vi.fn(),
  parents: vi.fn(), accounts: vi.fn(), proxies: vi.fn(), pools: vi.fn(), showError: vi.fn(), showSuccess: vi.fn()
}))
const route = reactive({ query: {} as Record<string, string> })
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...(await importOriginal<typeof import('vue-i18n')>()), useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  accountGroups: { list: mocks.list, getById: mocks.get, create: mocks.create, update: mocks.update, createAccount: mocks.createAccount, remove: mocks.remove },
  groups: { getAllIncludingInactive: mocks.parents }, accounts: { list: mocks.accounts },
  proxies: { getAll: mocks.proxies }, proxyPools: { list: mocks.pools }
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }) }))

const Layout = defineComponent({ setup(_, { slots }) { return () => h('div', [slots.default?.(), slots.filters?.(), slots.table?.()]) } })
const Dialog = defineComponent({ props: ['show', 'closeOnEscape'], setup(props, { slots }) { return () => props.show ? h('div', [slots.default?.(), slots.footer?.()]) : null } })
const Confirmation = defineComponent({
  name: 'ConfirmDialog', props: ['show', 'title', 'message', 'confirmText', 'cancelText', 'danger'],
  emits: ['confirm', 'cancel'],
  setup(props, { emit, slots }) {
    return () => props.show ? h('div', { 'data-testid': 'confirmation-dialog' }, [
      h('h2', props.title), h('p', props.message), slots.default?.(),
      h('button', { 'data-testid': 'confirm-overwrite', onClick: () => emit('confirm') }, props.confirmText),
      h('button', { 'data-testid': 'cancel-overwrite', onClick: () => emit('cancel') }, props.cancelText)
    ]) : null
  }
})
const Table = defineComponent({
  props: ['data'], setup(props, { slots }) { return () => h('div', (props.data ?? []).map((row: Record<string, unknown>) => h('div', slots['cell-actions']?.({ row })))) }
})
const Editor = defineComponent({
  name: 'EditAccountModal', props: ['show', 'account', 'accountConfigGroup', 'groups', 'proxies', 'pools'],
  emits: ['close', 'group-updated'],
  setup() { return () => h('div', { 'data-testid': 'typed-settings-editor' }) }
})
const fixture = {
  id: 10, name: 'Shared group', group_id: 5, platform: 'openai', type: 'oauth', account_ids: [1],
  config: { concurrency: 3, priority: 1, rate_multiplier: 1, status: 'active', schedulable: true, credentials: { model_mapping: { one: 'two' } }, extra: { base_rpm: 12 } }
}
const members = [
  { id: 1, name: 'Existing', group_ids: [5], platform: 'openai', type: 'oauth' },
  { id: 2, name: 'Available', group_ids: [5], platform: 'openai', type: 'oauth' },
  { id: 3, name: 'Different', group_ids: [5], platform: 'openai', type: 'apikey' },
  { id: 4, name: 'Also available', group_ids: [5], platform: 'openai', type: 'oauth' },
  { id: 5, name: 'Shadow', group_ids: [5], platform: 'openai', type: 'oauth', parent_account_id: 1 }
]
function render() {
  return mount(AccountGroupsView, { global: { stubs: { AppLayout: Layout, TablePageLayout: Layout, DataTable: Table, BaseDialog: Dialog, ConfirmDialog: Confirmation, EmptyState: true, Icon: true, EditAccountModal: Editor } } })
}

beforeEach(() => {
  vi.clearAllMocks()
  route.query = {}
  mocks.list.mockResolvedValue([fixture])
  mocks.get.mockResolvedValue(fixture)
  mocks.parents.mockResolvedValue([{ id: 5, name: 'Parent', platform: 'openai' }, { id: 6, name: 'Composite', platform: 'composite' }])
  mocks.accounts.mockResolvedValue({ items: members, pages: 1, total: 5 })
  mocks.proxies.mockResolvedValue([])
  mocks.pools.mockResolvedValue([])
  mocks.create.mockResolvedValue(fixture)
  mocks.update.mockResolvedValue(fixture)
  mocks.createAccount.mockResolvedValue({ id: 11, name: 'Shared group #11', account_config_group_id: 10 })
})

async function selectGroupOption(wrapper: VueWrapper, id: string, value: number) {
  const select = wrapper.findAllComponents(Select).find(component => component.props('id') === id)!
  select.vm.$emit('update:modelValue', value)
  await flushPromises()
}

describe('AccountGroupsView', () => {
  const apiKeyGroup = {
    ...fixture, type: 'apikey',
    config: { ...fixture.config, credentials: { ...fixture.config.credentials, base_url: 'https://default.example/v1' } }
  }
  async function prepareCreateAccount() {
    mocks.list.mockResolvedValue([apiKeyGroup])
    mocks.get.mockResolvedValue(apiKeyGroup)
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    return wrapper
  }

  it('offers quick creation only for API key and upstream account groups', async () => {
    mocks.list.mockResolvedValue([
      fixture,
      { ...apiKeyGroup, id: 11 },
      { ...fixture, id: 12, type: 'upstream' },
      { ...fixture, id: 13, type: 'service_account' },
      { ...fixture, id: 14, type: 'bedrock' }
    ])
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="create-account-group-member-10"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-account-group-member-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-account-group-member-12"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-account-group-member-13"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-account-group-member-14"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('loads the latest default address without member secrets and creates using only the API key', async () => {
    mocks.list.mockResolvedValue([apiKeyGroup])
    const freshURL = 'https://fresh.example/v1'
    mocks.get.mockResolvedValue({ ...apiKeyGroup, config: { ...apiKeyGroup.config, credentials: { base_url: freshURL } } })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    expect(mocks.get).toHaveBeenCalledWith(10)
    expect(mocks.accounts).not.toHaveBeenCalled()
    expect(wrapper.get('#account-group-member-form').findAll('input')).toHaveLength(2)
    expect(wrapper.get<HTMLInputElement>('#account-group-member-base-url').element.value).toBe(freshURL)
    const keyInput = wrapper.get<HTMLInputElement>('#account-group-member-api-key')
    expect(keyInput.element.value).toBe('')
    expect(keyInput.attributes()).toMatchObject({ type: 'password', autocomplete: 'new-password' })
    await keyInput.setValue(' new-member-key ')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(mocks.createAccount).toHaveBeenCalledWith(10, { api_key: 'new-member-key' }, expect.stringMatching(/^account-group-member-10-/))
    expect(mocks.update).not.toHaveBeenCalled()
    expect(mocks.create).not.toHaveBeenCalled()
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(mocks.showSuccess).toHaveBeenCalledWith('admin.accountGroups.accountCreated')
    expect(wrapper.find('#account-group-member-form').exists()).toBe(false)
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('#account-group-member-api-key').element.value).toBe('')
    wrapper.unmount()
  })

  it('sends a custom address only to the new member, without editing the group', async () => {
    const wrapper = await prepareCreateAccount()
    await wrapper.get('#account-group-member-base-url').setValue('https://independent.example/v1')
    await wrapper.get('#account-group-member-api-key').setValue('independent-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(mocks.createAccount).toHaveBeenCalledWith(10, { api_key: 'independent-key', base_url: 'https://independent.example/v1' }, expect.any(String))
    expect(mocks.update).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('uses the group default when the address is left blank', async () => {
    const wrapper = await prepareCreateAccount()
    await wrapper.get('#account-group-member-base-url').setValue('')
    await wrapper.get('#account-group-member-api-key').setValue('new-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(mocks.createAccount).toHaveBeenCalledWith(10, { api_key: 'new-key' }, expect.any(String))
    wrapper.unmount()
  })

  it('uses the effective server endpoint for adaptive groups without base_url', async () => {
    const group = {
      ...apiKeyGroup, default_base_url: 'https://adaptive.example/v1',
      config: { ...apiKeyGroup.config, credentials: { api_base_urls: { chat_completions: 'https://adaptive.example/v1' } } }
    }
    mocks.list.mockResolvedValue([group])
    mocks.get.mockResolvedValue(group)
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('#account-group-member-base-url').element.value).toBe(group.default_base_url)
    await wrapper.get('#account-group-member-api-key').setValue('adaptive-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(mocks.createAccount).toHaveBeenCalledWith(10, { api_key: 'adaptive-key' }, expect.any(String))
    wrapper.unmount()
  })

  it('requires a key and rejects invalid API addresses before sending', async () => {
    const wrapper = await prepareCreateAccount()
    expect(wrapper.get('[data-testid="save-account-group-member"]').attributes('disabled')).toBeDefined()
    await wrapper.get('#account-group-member-form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toBe('admin.accountGroups.apiKeyRequired')
    await wrapper.get('#account-group-member-api-key').setValue('new-key')
    for (const invalid of ['invalid', 'ftp://invalid.example', 'https://user:secret@invalid.example', 'https://invalid.example?key=secret', 'https://invalid.example#fragment']) {
      await wrapper.get('#account-group-member-base-url').setValue(invalid)
      await wrapper.get('#account-group-member-form').trigger('submit')
      expect(wrapper.get('[role="alert"]').text()).toBe('admin.accountGroups.apiAddressInvalid')
    }
    expect(mocks.createAccount).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('retains user input on creation errors and clears secrets when closed', async () => {
    mocks.createAccount.mockRejectedValueOnce({ message: 'Could not create account' })
    const wrapper = await prepareCreateAccount()
    await wrapper.get('#account-group-member-api-key').setValue('retry-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Could not create account')
    expect(wrapper.get<HTMLInputElement>('#account-group-member-api-key').element.value).toBe('retry-key')
    expect(mocks.list).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-testid="cancel-account-group-member"]').trigger('click')
    expect(wrapper.find('#account-group-member-form').exists()).toBe(false)
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('#account-group-member-api-key').element.value).toBe('')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('prevents duplicate creation and closing while a create request is pending', async () => {
    let finish!: (value: unknown) => void
    mocks.createAccount.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = await prepareCreateAccount()
    await wrapper.get('#account-group-member-api-key').setValue('new-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await wrapper.get('#account-group-member-form').trigger('submit')
    expect(mocks.createAccount).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="save-account-group-member"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="cancel-account-group-member"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('#account-group-member-api-key').attributes('disabled')).toBeDefined()
    finish({ id: 11 })
    await flushPromises()
    expect(wrapper.find('#account-group-member-form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('reuses the operation key for unchanged retries and rotates it after changed input or reopening', async () => {
    mocks.createAccount.mockRejectedValue({ message: 'Request timed out' })
    const wrapper = await prepareCreateAccount()
    await wrapper.get('#account-group-member-api-key').setValue('first-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    const firstOperation = mocks.createAccount.mock.calls[0]![2]
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(mocks.createAccount.mock.calls[1]![2]).toBe(firstOperation)
    await wrapper.get('#account-group-member-api-key').setValue('changed-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    const changedOperation = mocks.createAccount.mock.calls[2]![2]
    expect(changedOperation).not.toBe(firstOperation)
    await wrapper.get('[data-testid="cancel-account-group-member"]').trigger('click')
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    await wrapper.get('#account-group-member-api-key').setValue('changed-key')
    await wrapper.get('#account-group-member-form').trigger('submit')
    await flushPromises()
    expect(mocks.createAccount.mock.calls[3]![2]).not.toBe(changedOperation)
    wrapper.unmount()
  })

  it('does not open a quick-create dialog when current group settings fail to load', async () => {
    mocks.list.mockResolvedValue([apiKeyGroup])
    mocks.get.mockRejectedValueOnce({ message: 'Group no longer exists' })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group-member-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('#account-group-member-form').exists()).toBe(false)
    expect(mocks.showError).toHaveBeenCalledWith('Group no longer exists')
    expect(mocks.createAccount).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('enforces exclusive membership and same platform/type when creating', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group"]').trigger('click')
    await wrapper.get('#account-group-name').setValue('New group')
    const select = wrapper.findAllComponents(Select).find(component => component.props('id') === 'account-group-parent')!
    // Composite routing groups may also own compatible account groups.
    expect(select.props('options')).toHaveLength(3)
    await selectGroupOption(wrapper, 'account-group-parent', 5)
    await flushPromises()
    expect(mocks.accounts).toHaveBeenCalledWith(1, 100, { group: '5', lite: '1' })
    expect(wrapper.get('[data-testid="member-1"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="member-5"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="member-2"]').setValue(true)
    expect(wrapper.get('[data-testid="member-3"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="member-4"]').setValue(true)
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith({ name: 'New group', group_id: 5, account_ids: [2, 4], source_account_id: 2 })
    expect(wrapper.findComponent(Editor).props('accountConfigGroup')).toEqual(fixture)
    wrapper.unmount()
  })

  it('loads all candidate pages', async () => {
    mocks.accounts.mockResolvedValueOnce({ items: members.slice(0, 2), pages: 2 }).mockResolvedValueOnce({ items: members.slice(2), pages: 2 })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group"]').trigger('click')
    await selectGroupOption(wrapper, 'account-group-parent', 5)
    await flushPromises()
    expect(mocks.accounts).toHaveBeenNthCalledWith(2, 2, 100, { group: '5', lite: '1' })
    expect(wrapper.findAll('[data-testid^="member-"]')).toHaveLength(5)
    wrapper.unmount()
  })

  it('opens the platform-specific settings editor from query links without loading member credentials', async () => {
    route.query = { edit: '10' }
    const wrapper = render()
    await flushPromises()
    expect(mocks.get).toHaveBeenCalledWith(10)
    const editor = wrapper.findComponent(Editor)
    expect(editor.exists()).toBe(true)
    expect(editor.props('accountConfigGroup')).toEqual(fixture)
    expect(editor.props('account')).toMatchObject({
      platform: 'openai', type: 'oauth', credentials: fixture.config.credentials, concurrency: 3
    })
    expect(mocks.accounts).not.toHaveBeenCalled()
    expect(wrapper.find('#account-group-form').exists()).toBe(false)
    editor.vm.$emit('group-updated', fixture)
    await flushPromises()
    expect(wrapper.findComponent(Editor).exists()).toBe(false)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('provides separate membership editing without overwriting shared configuration', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="edit-account-group-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('#account-group-parent').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="member-1"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="member-2"]').setValue(true)
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(10, { name: 'Shared group', account_ids: [1, 2] })
    expect(wrapper.findComponent(Editor).exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens account settings using the current server configuration', async () => {
    const fresh = { ...fixture, type: 'apikey', config: { ...fixture.config, concurrency: 8 } }
    mocks.get.mockResolvedValueOnce(fresh)
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="account-group-settings-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(Editor).props('account')).toMatchObject({ type: 'apikey', concurrency: 8 })
    wrapper.findComponent(Editor).vm.$emit('close')
    await flushPromises()
    expect(wrapper.findComponent(Editor).exists()).toBe(false)
    wrapper.unmount()
  })

  const mappingConflict = {
    status: 409, code: 409, reason: 'ACCOUNT_CONFIG_GROUP_MAPPING_CONFLICT',
    message: 'Member model mappings differ',
    metadata: { account_id: '4', field: 'model_mapping', existing_entries: '7', target_entries: '1', removed_entries: '6', changed_entries: '1' }
  }
  async function prepareCreate() {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="create-account-group"]').trigger('click')
    await wrapper.get('#account-group-name').setValue('New group')
    await selectGroupOption(wrapper, 'account-group-parent', 5)
    await flushPromises()
    await wrapper.get('[data-testid="member-2"]').setValue(true)
    await wrapper.get('[data-testid="member-4"]').setValue(true)
    return wrapper
  }

  it('warns before replacing create-member mappings and lets users cancel and change the source', async () => {
    mocks.create.mockRejectedValueOnce(mappingConflict)
    const wrapper = await prepareCreate()
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledTimes(1)
    expect(mocks.create.mock.calls[0]![0]).not.toHaveProperty('confirm_model_mapping_overwrite')
    expect(wrapper.find('#account-group-form').exists()).toBe(true)
    const warning = wrapper.get('[data-testid="confirmation-dialog"]')
    expect(warning.text()).toContain('admin.accountGroups.mappingConflictCreate')
    expect(wrapper.findComponent(Dialog).props('closeOnEscape')).toBe(false)
    expect(warning.text()).toContain('Available (#2)')
    expect(warning.text()).toContain('Also available (#4)')
    expect(warning.text()).toContain('"removed":"6"')
    await wrapper.get('[data-testid="cancel-overwrite"]').trigger('click')
    expect(wrapper.findComponent(Dialog).props('closeOnEscape')).toBe(true)
    expect(wrapper.find('[data-testid="confirmation-dialog"]').exists()).toBe(false)
    expect(wrapper.get('#account-group-name').element.value).toBe('New group')
    expect(wrapper.get('[data-testid="member-4"]').element.checked).toBe(true)
    await selectGroupOption(wrapper, 'account-group-source', 4)
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenLastCalledWith({ name: 'New group', group_id: 5, account_ids: [2, 4], source_account_id: 4 })
    wrapper.unmount()
  })

  it('sends an overwrite flag only after explicit create confirmation and consumes it after an error', async () => {
    mocks.create.mockRejectedValueOnce(mappingConflict).mockRejectedValueOnce({ message: 'Temporary save failure' })
    const wrapper = await prepareCreate()
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-testid="confirm-overwrite"]').trigger('click')
    await flushPromises()
    expect(mocks.create).toHaveBeenLastCalledWith({ name: 'New group', group_id: 5, account_ids: [2, 4], source_account_id: 2, confirm_model_mapping_overwrite: true })
    expect(wrapper.get('[role="alert"]').text()).toBe('Temporary save failure')
    expect(wrapper.find('[data-testid="confirmation-dialog"]').exists()).toBe(false)
    expect(wrapper.find('#account-group-form').exists()).toBe(true)
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenLastCalledWith({ name: 'New group', group_id: 5, account_ids: [2, 4], source_account_id: 2 })
    wrapper.unmount()
  })

  it('requires separate confirmation to replace mappings when joining an existing group', async () => {
    mocks.update.mockRejectedValueOnce({ ...mappingConflict, metadata: { ...mappingConflict.metadata, field: 'compact_model_mapping' } })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="edit-account-group-10"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="member-4"]').setValue(true)
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledTimes(1)
    const warning = wrapper.get('[data-testid="confirmation-dialog"]')
    expect(warning.text()).toContain('admin.accountGroups.mappingConflictJoin')
    expect(warning.text()).toContain('Shared group')
    expect(warning.text()).toContain('admin.accountGroups.mappingConflictCompactField')
    await wrapper.get('[data-testid="confirm-overwrite"]').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenLastCalledWith(10, { name: 'Shared group', account_ids: [1, 4], confirm_model_mapping_overwrite: true })
    expect(wrapper.find('#account-group-form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('invalidates a pending confirmation when the submitted member selection changes', async () => {
    mocks.create.mockRejectedValueOnce(mappingConflict)
    const wrapper = await prepareCreate()
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[data-testid="confirmation-dialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="member-4"]').setValue(false)
    expect(wrapper.find('[data-testid="confirmation-dialog"]').exists()).toBe(false)
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenLastCalledWith({ name: 'New group', group_id: 5, account_ids: [2], source_account_id: 2 })
    wrapper.unmount()
  })

  it('keeps membership editing open on validation conflicts', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="edit-account-group-10"]').trigger('click')
    await flushPromises()
    mocks.update.mockRejectedValueOnce({ message: 'Concurrent membership change' })
    await wrapper.get('#account-group-form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Concurrent membership change')
    expect(wrapper.find('#account-group-form').exists()).toBe(true)
    wrapper.unmount()
  })
})
