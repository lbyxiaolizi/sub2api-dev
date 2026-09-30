package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type configGroupAccountsStub struct {
	AccountRepository
	accounts   map[int64]*Account
	groups     map[int64]*AccountConfigGroup
	membership map[int64]int64
	saves      int
}

func (r *configGroupAccountsStub) GetByID(_ context.Context, id int64) (*Account, error) {
	if a := r.accounts[id]; a != nil {
		return a, nil
	}
	return nil, ErrAccountNotFound
}
func (r *configGroupAccountsStub) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	out := []*Account{}
	for _, id := range ids {
		if a := r.accounts[id]; a != nil {
			out = append(out, a)
		}
	}
	return out, nil
}
func (r *configGroupAccountsStub) GetAccountConfigGroup(_ context.Context, id int64) (*AccountConfigGroup, error) {
	if g := r.groups[id]; g != nil {
		clone := *g
		return &clone, nil
	}
	return nil, ErrAccountConfigGroupNotFound
}
func (r *configGroupAccountsStub) GetAccountConfigGroupByAccount(ctx context.Context, id int64) (*AccountConfigGroup, error) {
	if gid := r.membership[id]; gid != 0 {
		return r.GetAccountConfigGroup(ctx, gid)
	}
	return nil, nil
}
func (r *configGroupAccountsStub) ListAccountConfigGroups(context.Context, int64) ([]AccountConfigGroup, error) {
	return nil, nil
}
func (r *configGroupAccountsStub) DeleteAccountConfigGroup(context.Context, int64) error { return nil }
func (r *configGroupAccountsStub) SaveAccountConfigGroup(_ context.Context, g *AccountConfigGroup) error {
	for _, id := range g.AccountIDs {
		if gid := r.membership[id]; gid != 0 && gid != g.ID {
			return ErrAccountConfigGroupConflict
		}
	}
	if g.ID == 0 {
		g.ID = int64(len(r.groups) + 1)
	}
	r.groups[g.ID] = g
	for _, id := range g.AccountIDs {
		r.membership[id] = g.ID
	}
	r.saves++
	return nil
}

type configGroupParentsStub struct{ GroupRepository }

func (configGroupParentsStub) GetByID(context.Context, int64) (*Group, error) {
	return &Group{ID: 7}, nil
}

func configGroupTestService() (*adminServiceImpl, *configGroupAccountsStub) {
	repo := &configGroupAccountsStub{accounts: map[int64]*Account{}, groups: map[int64]*AccountConfigGroup{}, membership: map[int64]int64{}}
	for _, id := range []int64{1, 2, 3} {
		repo.accounts[id] = &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, GroupIDs: []int64{7}, Concurrency: 3, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "private", "base_url": "https://example.test"}}
	}
	return &adminServiceImpl{accountRepo: repo, groupRepo: configGroupParentsStub{}}, repo
}
func TestAccountConfigGroupRequiresSameParentAndCompatibleAccounts(t *testing.T) {
	for _, change := range []func(*Account){func(a *Account) { a.GroupIDs = []int64{8} }, func(a *Account) { a.Platform = PlatformAnthropic }, func(a *Account) { a.Type = AccountTypeOAuth }, func(a *Account) { v := int64(10); a.ParentAccountID = &v; a.QuotaDimension = "spark" }} {
		svc, repo := configGroupTestService()
		change(repo.accounts[2])
		_, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{Name: "group", GroupID: 7, AccountIDs: []int64{1, 2}})
		require.ErrorIs(t, err, ErrAccountConfigGroupInvalid)
		require.Zero(t, repo.saves)
	}
}
func TestAccountConfigGroupExclusiveMembership(t *testing.T) {
	svc, repo := configGroupTestService()
	first, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{Name: "first", GroupID: 7, AccountIDs: []int64{1, 2}})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, first.AccountIDs)
	_, err = svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{Name: "second", GroupID: 7, AccountIDs: []int64{2, 3}})
	require.ErrorIs(t, err, ErrAccountConfigGroupConflict)
	require.Equal(t, 1, repo.saves)
}
func TestAccountConfigGroupConfigPatchPreservesUntouchedFields(t *testing.T) {
	svc, _ := configGroupTestService()
	group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{Name: "first", GroupID: 7, AccountIDs: []int64{1, 2}})
	require.NoError(t, err)
	updated, err := svc.UpdateAccountConfigGroup(context.Background(), group.ID, &UpdateAccountConfigGroupInput{Config: json.RawMessage(`{"concurrency":0,"rate_multiplier":0,"priority":0,"schedulable":false}`)})
	require.NoError(t, err)
	require.Zero(t, updated.Config.Concurrency)
	require.Zero(t, updated.Config.RateMultiplier)
	require.False(t, updated.Config.Schedulable)
	require.Equal(t, "https://example.test", updated.Config.Credentials["base_url"])
	_, err = svc.UpdateAccountConfigGroup(context.Background(), group.ID, &UpdateAccountConfigGroupInput{Config: json.RawMessage(`{"credentials":{"api_key":"overwrite"}}`)})
	require.Error(t, err)
}
func TestAccountConfigGroupPreservesAuthenticationAndRuntimeState(t *testing.T) {
	creds := map[string]any{"api_key": "token-one", "email": "one@example.test", "account_id": "one", "header_override_enabled": true, "model_mapping": map[string]any{"a": "b"}}
	settings := AccountConfigGroupCredentialSettings(creds)
	require.NotContains(t, settings, "api_key")
	require.NotContains(t, settings, "email")
	require.NotContains(t, settings, "account_id")
	result := MergeAccountConfigGroupSettings(map[string]any{"api_key": "token-two", "email": "two@example.test", "base_url": "old"}, settings, true)
	require.Equal(t, "token-two", result["api_key"])
	require.Equal(t, "two@example.test", result["email"])
	require.NotContains(t, result, "base_url")
	require.Equal(t, true, result["header_override_enabled"])
	extra := map[string]any{"quota_used": 19.0, "quota_limit": 30.0, "privacy_mode": "private", "codex_fingerprint_seed": "unique", "future_provider_observation": true}
	result = MergeAccountConfigGroupSettings(extra, map[string]any{"quota_limit": 50.0}, false)
	require.Equal(t, 19.0, result["quota_used"])
	require.Equal(t, 50.0, result["quota_limit"])
	require.Equal(t, "unique", result["codex_fingerprint_seed"])
	require.Equal(t, true, result["future_provider_observation"])
}
func TestAccountConfigGroupDirectUpdateGuards(t *testing.T) {
	svc, repo := configGroupTestService()
	ctx := context.Background()
	_, err := svc.CreateAccountConfigGroup(ctx, &CreateAccountConfigGroupInput{Name: "first", GroupID: 7, AccountIDs: []int64{1, 2}})
	require.NoError(t, err)
	value := 4
	_, err = svc.UpdateAccount(ctx, 1, &UpdateAccountInput{Concurrency: &value})
	require.ErrorIs(t, err, ErrAccountConfigGroupManaged)
	_, err = svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{AccountIDs: []int64{3, 1}, Concurrency: &value})
	require.ErrorIs(t, err, ErrAccountConfigGroupManaged)
	require.Equal(t, 3, repo.accounts[3].Concurrency)
	_, err = svc.SetAccountSchedulable(ctx, 1, false)
	require.ErrorIs(t, err, ErrAccountConfigGroupManaged)
	req := &UpdateAccountInput{Credentials: map[string]any{"api_key": "new"}}
	require.NoError(t, svc.guardAccountConfigGroupUpdate(ctx, repo.accounts[1], req))
	require.Equal(t, "https://example.test", req.Credentials["base_url"])
	ids := []int64{8}
	require.ErrorIs(t, svc.guardAccountConfigGroupUpdate(ctx, repo.accounts[1], &UpdateAccountInput{GroupIDs: &ids}), ErrAccountConfigGroupManaged)
}
func TestAccountConfigGroupConfigCreationClonesExpiryAndExcludesRuntime(t *testing.T) {
	now := time.Now()
	config := accountConfigFromAccount(&Account{ExpiresAt: &now, Extra: map[string]any{"quota_daily_used": 3, "quota_daily_limit": 10, "codex_auto_reset_credit_state": map[string]any{"x": 1}}})
	require.Equal(t, now.Unix(), *config.ExpiresAt)
	require.NotContains(t, config.Extra, "quota_daily_used")
	require.NotContains(t, config.Extra, "codex_auto_reset_credit_state")
	require.Equal(t, 10, config.Extra["quota_daily_limit"])
}
