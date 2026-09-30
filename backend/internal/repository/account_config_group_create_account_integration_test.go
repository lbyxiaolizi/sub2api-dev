//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateAccountInConfigGroupAtomicMembershipAndIndependentEndpoint(t *testing.T) {
	ctx := context.Background()
	repo, parent, members := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, members[0].ID, members[1].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	created := &service.Account{Name: fmt.Sprintf("config-created-%d", time.Now().UnixNano()), Platform: group.Platform, Type: group.Type,
		Credentials: map[string]any{"api_key": "new-independent-key", service.AccountConfigGroupBaseURLOverrideKey: "https://private.example/v1"},
		Extra:       map[string]any{}, Status: service.StatusActive, Schedulable: true}
	require.NoError(t, repo.CreateAccountInConfigGroup(ctx, group, created))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_config_group_members WHERE account_id=$1`, created.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduler_outbox WHERE account_id=$1`, created.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=$1`, created.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id=$1`, created.ID)
	})
	require.Equal(t, []int64{parent.ID}, created.GroupIDs)
	require.Equal(t, 9, created.Concurrency)
	require.Equal(t, group.Config.RateMultiplier, created.BillingRateMultiplier())
	require.Equal(t, "new-independent-key", created.Credentials["api_key"])
	require.Equal(t, "https://private.example/v1", created.Credentials["base_url"])
	require.Equal(t, group.Config.Credentials["model_mapping"], created.Credentials["model_mapping"])
	require.NotContains(t, created.Extra, "quota_daily_used")
	require.NotContains(t, created.Extra, "privacy_mode")
	joined, err := repo.GetAccountConfigGroupByAccount(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, group.ID, joined.ID)
	require.Len(t, joined.AccountIDs, 3)
	var outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_outbox WHERE account_id=$1`, created.ID).Scan(&outboxCount))
	require.Positive(t, outboxCount)

	group.Config.Concurrency = 17
	group.Config.Credentials["base_url"] = "https://new-default.example"
	group.Config.Credentials["model_mapping"] = map[string]any{"new-alias": "new-model"}
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	// A partial credential replacement must recover the endpoint from the locked
	// stored marker, not from this request's absent or spoofed marker.
	require.NoError(t, repo.UpdateCredentials(ctx, created.ID, map[string]any{"api_key": "rotated-key", service.AccountConfigGroupBaseURLOverrideKey: "https://wrong.example"}))
	current, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "https://private.example/v1", current.Credentials["base_url"])
	require.Equal(t, "rotated-key", current.Credentials["api_key"])
	require.Equal(t, 17, current.Concurrency)
	require.Equal(t, group.Config.Credentials["model_mapping"], current.Credentials["model_mapping"])
	// Full stale snapshots receive exactly the same locked endpoint protection.
	delete(current.Credentials, service.AccountConfigGroupBaseURLOverrideKey)
	current.Credentials["base_url"] = "https://stale.example"
	require.NoError(t, repo.Update(ctx, current))
	current, err = repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "https://private.example/v1", current.Credentials["base_url"])
	source, err := repo.GetByID(ctx, members[0].ID)
	require.NoError(t, err)
	require.Equal(t, "identity-0", source.Credentials["api_key"])
	require.Equal(t, "https://new-default.example", source.Credentials["base_url"])
}

func TestCreateAccountInConfigGroupStaleSnapshotRollsBackCreatedAccount(t *testing.T) {
	ctx := context.Background()
	repo, parent, members := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, members[0].ID)
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	stale := *group
	group.Config.Concurrency = 29
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	created := &service.Account{Name: fmt.Sprintf("config-rollback-%d", time.Now().UnixNano()), Platform: group.Platform, Type: group.Type,
		Credentials: map[string]any{"api_key": "rollback-key"}, Extra: map[string]any{}, Status: service.StatusActive, Schedulable: true}
	var beforeOutbox int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_outbox`).Scan(&beforeOutbox))
	require.ErrorIs(t, repo.CreateAccountInConfigGroup(ctx, &stale, created), service.ErrAccountConfigGroupStale)
	require.Zero(t, created.ID)
	var count, afterOutbox int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE name=$1`, created.Name).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_outbox`).Scan(&afterOutbox))
	require.Equal(t, beforeOutbox, afterOutbox)
	loaded, err := repo.GetAccountConfigGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{members[0].ID}, loaded.AccountIDs)
	require.Equal(t, 29, loaded.Config.Concurrency)
}

func TestCreateAccountInConfigGroupEndpointReleasedOnRemovalOrDissolve(t *testing.T) {
	for _, dissolve := range []bool{false, true} {
		t.Run(fmt.Sprintf("dissolve=%t", dissolve), func(t *testing.T) {
			ctx := context.Background()
			repo, parent, members := accountConfigGroupFixture(t)
			members[0].Platform = service.PlatformOpenCodeGo
			require.NoError(t, repo.Update(ctx, members[0]))
			group := testAccountConfigGroup(parent.ID, members[0].ID)
			group.Platform = service.PlatformOpenCodeGo
			group.Config.Credentials["api_protocol"] = service.APIProtocolAdaptive
			require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
			created := &service.Account{Name: fmt.Sprintf("config-release-%d", time.Now().UnixNano()), Platform: group.Platform, Type: group.Type,
				Credentials: map[string]any{"api_key": "member-key", service.AccountConfigGroupBaseURLOverrideKey: "https://private.example/v1"},
				Extra:       map[string]any{}, Status: service.StatusActive, Schedulable: true}
			require.NoError(t, repo.CreateAccountInConfigGroup(ctx, group, created))
			t.Cleanup(func() {
				_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_config_group_members WHERE account_id=$1`, created.ID)
				_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduler_outbox WHERE account_id=$1`, created.ID)
				_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=$1`, created.ID)
				_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id=$1`, created.ID)
			})
			if dissolve {
				require.NoError(t, repo.DeleteAccountConfigGroup(ctx, group.ID))
			} else {
				group.AccountIDs = []int64{members[0].ID}
				require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
			}
			current, err := repo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			require.NotContains(t, current.Credentials, service.AccountConfigGroupBaseURLOverrideKey)
			require.Equal(t, "https://private.example/v1", current.GetOpenAIBaseURL())
			// A stale pre-detach full update must not resurrect the marker.
			created.Credentials["base_url"] = "https://edited.example/v1"
			created.Credentials["api_base_urls"] = map[string]any{
				service.APIProtocolChatCompletions: "https://edited.example/v1",
				service.APIProtocolAnthropic:       "https://edited.example",
			}
			require.NoError(t, repo.Update(ctx, created))
			current, err = repo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			require.NotContains(t, current.Credentials, service.AccountConfigGroupBaseURLOverrideKey)
			require.Equal(t, "https://edited.example/v1", current.GetOpenAIBaseURL())
			require.Equal(t, "https://edited.example", current.GetAnthropicProtocolBaseURL())
			current.Credentials[service.AccountConfigGroupBaseURLOverrideKey] = "https://stale.example"
			require.NoError(t, repo.UpdateCredentials(ctx, created.ID, current.Credentials))
			current, err = repo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			require.NotContains(t, current.Credentials, service.AccountConfigGroupBaseURLOverrideKey)
			require.Equal(t, "https://edited.example", current.GetAnthropicProtocolBaseURL())
		})
	}
}
