//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func accountConfigGroupFixture(t *testing.T) (*accountRepository, *service.Group, []*service.Account) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	suffix := time.Now().UnixNano()
	parent, err := client.Group.Create().SetName(fmt.Sprintf("config-group-parent-%d", suffix)).SetPlatform(service.PlatformOpenAI).Save(ctx)
	require.NoError(t, err)
	accounts := make([]*service.Account, 3)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_config_groups WHERE group_id=$1`, parent.ID)
		for _, account := range accounts {
			if account == nil {
				continue
			}
			_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduler_outbox WHERE account_id=$1`, account.ID)
			_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=$1`, account.ID)
			_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id=$1`, account.ID)
		}
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, parent.ID)
	})
	for i := range accounts {
		accounts[i] = &service.Account{
			Name: fmt.Sprintf("config-member-%d-%d", suffix, i), Platform: service.PlatformOpenAI,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
			Concurrency: 3, Priority: 50,
			Credentials: map[string]any{"api_key": fmt.Sprintf("identity-%d", i), "base_url": "https://old.example", "model_mapping": map[string]any{"old": "old"}},
			Extra:       map[string]any{"quota_daily_used": float64(i + 1), "privacy_mode": "training_off", "custom_base_url": "https://old-extra.example"},
		}
		require.NoError(t, repo.CreateWithAccountGroups(ctx, accounts[i], []service.AccountGroup{{GroupID: parent.ID, Priority: 1}}))
	}
	return repo, &service.Group{ID: parent.ID, Platform: parent.Platform}, accounts
}

func testAccountConfigGroup(parentID int64, accounts ...int64) *service.AccountConfigGroup {
	return &service.AccountConfigGroup{
		Name: "shared configuration", GroupID: parentID, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, AccountIDs: accounts,
		// This fixture deliberately replaces the members' {old: old} mapping.
		ConfirmModelMappingOverwrite: true,
		Config: service.AccountConfigGroupConfig{
			Concurrency: 9, Priority: 2, RateMultiplier: 1.25, Status: service.StatusActive, Schedulable: true, AutoPauseOnExpired: true,
			Credentials: map[string]any{"base_url": "https://shared.example", "model_mapping": map[string]any{"client": "upstream"}},
			Extra:       map[string]any{"custom_base_url": "https://shared-extra.example"},
		},
	}
}

func TestAccountConfigGroupSynchronizesSettingsPreservesIdentityAndRuntime(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	require.Positive(t, group.ID)
	for i := 0; i < 2; i++ {
		current, err := repo.GetByID(ctx, accounts[i].ID)
		require.NoError(t, err)
		require.Equal(t, 9, current.Concurrency)
		require.Equal(t, 2, current.Priority)
		require.Equal(t, 1.25, current.BillingRateMultiplier())
		require.Equal(t, fmt.Sprintf("identity-%d", i), current.Credentials["api_key"])
		require.Equal(t, "https://shared.example", current.Credentials["base_url"])
		require.Equal(t, float64(i+1), current.Extra["quota_daily_used"])
		require.Equal(t, "training_off", current.Extra["privacy_mode"])
		require.Equal(t, "https://shared-extra.example", current.Extra["custom_base_url"])
	}
	outsider, err := repo.GetByID(ctx, accounts[2].ID)
	require.NoError(t, err)
	require.Equal(t, 3, outsider.Concurrency)
	group.Config.Credentials = map[string]any{}
	group.Config.Extra = map[string]any{}
	group.Config.Concurrency = 17
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.NotContains(t, current.Credentials, "base_url")
	require.NotContains(t, current.Credentials, "model_mapping")
	require.NotContains(t, current.Extra, "custom_base_url")
	require.Equal(t, "identity-0", current.Credentials["api_key"])
	require.Equal(t, float64(1), current.Extra["quota_daily_used"])
	require.Equal(t, 17, current.Concurrency)
	loaded, err := repo.GetAccountConfigGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, group.AccountIDs, loaded.AccountIDs)
	require.Equal(t, 17, loaded.Config.Concurrency)
}

func TestAccountConfigGroupTypedSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 2; i++ {
		accounts[i].Type = service.AccountTypeOAuth
		accounts[i].Credentials = map[string]any{"access_token": fmt.Sprintf("oauth-token-%d", i), "plan_type": "plus"}
		accounts[i].Extra["quota_daily_start"] = now
		require.NoError(t, repo.Update(ctx, accounts[i]))
	}
	group := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	group.Type = service.AccountTypeOAuth
	group.Config.Credentials = map[string]any{"plan_type": "pro"}
	group.Config.Extra = map[string]any{
		"codex_fingerprint_mode": "full", "quota_daily_limit": 100.0,
		"quota_daily_reset_mode": "fixed", "quota_daily_reset_hour": 0.0, "quota_reset_timezone": "UTC",
		"quota_notify_daily_enabled": true, "quota_notify_daily_threshold": 10.0,
		"quota_notify_daily_threshold_type": "percentage",
	}
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	seeds := make([]string, 2)
	for i := 0; i < 2; i++ {
		current, err := repo.GetByID(ctx, accounts[i].ID)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("oauth-token-%d", i), current.Credentials["access_token"])
		require.Equal(t, "pro", current.Credentials["plan_type"])
		require.Equal(t, float64(i+1), current.Extra["quota_daily_used"])
		require.Equal(t, "percentage", current.Extra["quota_notify_daily_threshold_type"])
		require.Contains(t, current.Extra, "quota_daily_reset_at")
		seeds[i], _ = current.Extra["codex_fingerprint_seed"].(string)
		require.NotEmpty(t, seeds[i])
		current.Credentials["access_token"] = fmt.Sprintf("refreshed-token-%d", i)
		current.Credentials["plan_type"] = "plus"
		require.NoError(t, repo.UpdateCredentials(ctx, current.ID, current.Credentials))
		refreshed, err := repo.GetByID(ctx, current.ID)
		require.NoError(t, err)
		require.Equal(t, "pro", refreshed.Credentials["plan_type"])
		require.Equal(t, fmt.Sprintf("refreshed-token-%d", i), refreshed.Credentials["access_token"])
	}
	require.NotEqual(t, seeds[0], seeds[1])
	require.NotContains(t, group.Config.Extra, "codex_fingerprint_seed")
	require.NotContains(t, group.Config.Extra, "quota_daily_reset_at")

	group.Config.Credentials = map[string]any{}
	group.Config.Extra = map[string]any{}
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	for i := 0; i < 2; i++ {
		current, err := repo.GetByID(ctx, accounts[i].ID)
		require.NoError(t, err)
		require.NotContains(t, current.Credentials, "plan_type")
		require.NotContains(t, current.Extra, "codex_fingerprint_mode")
		require.NotContains(t, current.Extra, "quota_notify_daily_threshold_type")
		require.NotContains(t, current.Extra, "quota_daily_reset_at")
		require.Equal(t, seeds[i], current.Extra["codex_fingerprint_seed"])
		require.Equal(t, float64(i+1), current.Extra["quota_daily_used"])
		current.Credentials["plan_type"] = "team"
		require.NoError(t, repo.UpdateCredentials(ctx, current.ID, current.Credentials))
	}
	group.Name = "automatic tier remains independent"
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	for i := 0; i < 2; i++ {
		current, err := repo.GetByID(ctx, accounts[i].ID)
		require.NoError(t, err)
		require.Equal(t, "team", current.Credentials["plan_type"])
	}
}

func TestAccountConfigGroupRejectsCrossParentAndConflictingMembershipAtomically(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	conflict := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	require.ErrorIs(t, repo.SaveAccountConfigGroup(ctx, conflict), service.ErrAccountConfigGroupConflict)
	ungrouped, err := repo.GetAccountConfigGroupByAccount(ctx, accounts[1].ID)
	require.NoError(t, err)
	require.Nil(t, ungrouped)
	require.NoError(t, repo.BindGroups(ctx, accounts[2].ID, nil))
	group.AccountIDs = append(group.AccountIDs, accounts[2].ID)
	group.Config.Concurrency = 77
	require.ErrorIs(t, repo.SaveAccountConfigGroup(ctx, group), service.ErrAccountConfigGroupInvalid)
	persisted, err := repo.GetAccountConfigGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{accounts[0].ID}, persisted.AccountIDs)
	require.Equal(t, 9, persisted.Config.Concurrency)
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, 9, current.Concurrency)
}

func TestAccountConfigGroupExclusiveMembershipConcurrentCreate(t *testing.T) {
	repo, parent, accounts := accountConfigGroupFixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- repo.SaveAccountConfigGroup(context.Background(), testAccountConfigGroup(parent.ID, accounts[0].ID))
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, service.ErrAccountConfigGroupConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
}

func TestAccountConfigGroupRoutingBindingConstraintAndDeletion(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	require.NoError(t, repo.BindGroups(ctx, accounts[0].ID, []int64{parent.ID}), "replacement of an unchanged binding must remain valid")
	require.Error(t, repo.BindGroups(ctx, accounts[0].ID, nil), "a grouped account must retain its parent binding")
	require.NoError(t, repo.Delete(ctx, accounts[0].ID), "account deletion must release protected membership atomically")
	loaded, err := repo.GetAccountConfigGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{accounts[1].ID}, loaded.AccountIDs)
	require.NoError(t, repo.DeleteAccountConfigGroup(ctx, group.ID))
	current, err := repo.GetByID(ctx, accounts[1].ID)
	require.NoError(t, err)
	require.Equal(t, 9, current.Concurrency, "dissolving a group retains its last applied configuration")
	require.NoError(t, repo.BindGroups(ctx, accounts[1].ID, nil))
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, testAccountConfigGroup(parent.ID, accounts[2].ID)))
	groupRepo := NewGroupRepository(testEntClient(t), integrationDB)
	_, err = groupRepo.DeleteCascade(ctx, parent.ID)
	require.NoError(t, err, "deleting the parent must release configuration groups before routing bindings")
	groups, err := repo.ListAccountConfigGroups(ctx, parent.ID)
	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestAccountConfigGroupRejectsStaleUpdate(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	stale, err := repo.GetAccountConfigGroup(ctx, group.ID)
	require.NoError(t, err)
	group.Config.Concurrency = 22
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	stale.Config.Concurrency = 33
	require.ErrorIs(t, repo.SaveAccountConfigGroup(ctx, stale), service.ErrAccountConfigGroupStale)
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, 22, current.Concurrency)
}

func TestAccountConfigGroupStaleAccountUpdatePreservesSharedConfiguration(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	stale, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	stale.Credentials["api_key"] = "rotated-identity"
	require.NoError(t, repo.Update(ctx, stale), "a token refresh based on a stale snapshot must not overwrite shared settings")
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, 9, current.Concurrency)
	require.Equal(t, 2, current.Priority)
	require.Equal(t, 1.25, current.BillingRateMultiplier())
	require.Equal(t, "https://shared.example", current.Credentials["base_url"])
	require.Equal(t, "rotated-identity", current.Credentials["api_key"])
	require.Equal(t, "https://shared-extra.example", current.Extra["custom_base_url"])
	require.NoError(t, repo.UpdateCredentials(ctx, accounts[0].ID, map[string]any{"api_key": "rotated-again", "base_url": "https://stale.example"}))
	current, err = repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, "rotated-again", current.Credentials["api_key"])
	require.Equal(t, "https://shared.example", current.Credentials["base_url"])
	peer, err := repo.GetByID(ctx, accounts[1].ID)
	require.NoError(t, err)
	require.Equal(t, "identity-1", peer.Credentials["api_key"], "identity changes remain account-specific")
}

func TestAccountConfigGroupRenamePreservesRuntimeFailures(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	require.NoError(t, repo.SetError(ctx, accounts[0].ID, "provider rejected refreshed token"))
	group.Name = "renamed without changing configuration"
	group.Config.Concurrency = 12
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusError, current.Status)
	require.False(t, current.Schedulable)
	require.Equal(t, 12, current.Concurrency)
	group.Config.Status = "inactive"
	group.Config.Schedulable = false
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	group.Config.Status = service.StatusActive
	group.Config.Schedulable = true
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	current, err = repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, current.Status)
	require.True(t, current.Schedulable)
}

func TestAccountConfigGroupPoolAssignmentsRemainPerAccount(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	client := testEntClient(t)
	pool, err := client.ProxyPool.Create().SetName(fmt.Sprintf("config-pool-%d", time.Now().UnixNano())).Save(ctx)
	require.NoError(t, err)
	proxies := make([]int64, 0, 2)
	t.Cleanup(func() {
		for _, id := range proxies {
			_, _ = integrationDB.ExecContext(ctx, `DELETE FROM proxies WHERE id=$1`, id)
		}
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM proxy_pools WHERE id=$1`, pool.ID)
	})
	for i := 0; i < 2; i++ {
		proxy, err := client.Proxy.Create().SetName(fmt.Sprintf("config-pool-proxy-%d-%d", pool.ID, i)).SetProtocol("http").SetHost("127.0.0.1").SetPort(18000 + i).SetStatus(service.StatusActive).SetPoolID(pool.ID).SetPoolHealth(service.PoolHealthHealthy).Save(ctx)
		require.NoError(t, err)
		proxies = append(proxies, proxy.ID)
	}
	group := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	group.Config.PoolID = &pool.ID
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	assignments := make([]int64, 2)
	for i := 0; i < 2; i++ {
		current, err := repo.GetByID(ctx, accounts[i].ID)
		require.NoError(t, err)
		require.NotNil(t, current.ProxyID)
		require.Equal(t, pool.ID, *current.PoolID)
		assignments[i] = *current.ProxyID
	}
	require.NotEqual(t, assignments[0], assignments[1], "initial assignment balances healthy proxies")
	group.Config.Concurrency = 13
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	for i := 0; i < 2; i++ {
		current, err := repo.GetByID(ctx, accounts[i].ID)
		require.NoError(t, err)
		require.Equal(t, assignments[i], *current.ProxyID, "unrelated group changes retain runtime pool assignment")
	}
}

func TestAccountConfigGroupInvalidatesEndpointBoundSnapshots(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET extra=extra || '{"upstream_billing_probe":{"old":true},"ollama_cloud_usage_session":{"old":true},"ollama_cloud_usage_snapshot":{"old":true},"opencode_go_usage_snapshot":{"old":true}}'::jsonb WHERE id=$1`, accounts[0].ID)
	require.NoError(t, err)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	for _, key := range []string{"upstream_billing_probe", "ollama_cloud_usage_session", "ollama_cloud_usage_snapshot", "opencode_go_usage_snapshot"} {
		require.NotContains(t, current.Extra, key)
	}
	require.Equal(t, float64(1), current.Extra["quota_daily_used"])
}
