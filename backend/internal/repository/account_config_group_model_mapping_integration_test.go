//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupModelMappingsSurviveMembershipConfigAndRefresh(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID)
	mapping := map[string]any{"client": "upstream", "client-*": "upstream-*"}
	compactMapping := map[string]any{"compact-client": "compact-upstream"}
	group.Config.Credentials["model_mapping"] = mapping
	group.Config.Credentials["compact_model_mapping"] = compactMapping
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))

	assertMappings := func() {
		t.Helper()
		persisted, err := repo.GetAccountConfigGroup(ctx, group.ID)
		require.NoError(t, err)
		require.Equal(t, mapping, persisted.Config.Credentials["model_mapping"])
		require.Equal(t, compactMapping, persisted.Config.Credentials["compact_model_mapping"])
		for _, id := range group.AccountIDs {
			current, err := repo.GetByID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, mapping, current.Credentials["model_mapping"])
			require.Equal(t, compactMapping, current.Credentials["compact_model_mapping"])
		}
	}
	assertMappings()

	// A membership-only save must use the group's full stored settings, rather
	// than reconstructing them from lightweight account-list projections.
	group.AccountIDs = append(group.AccountIDs, accounts[1].ID, accounts[2].ID)
	group.ConfirmModelMappingOverwrite = true // Explicitly replace the joining members' old mapping.
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	assertMappings()

	group.Config.Priority++
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	assertMappings()

	// Both refresh paths are allowed to omit all settings and supply just the
	// independent identity; they must reapply the latest group mapping.
	require.NoError(t, repo.UpdateCredentials(ctx, accounts[0].ID, map[string]any{"api_key": "rotated-private"}))
	assertMappings()
	stale := accounts[1]
	stale.Credentials = map[string]any{"api_key": "rotated-peer"}
	require.NoError(t, repo.Update(ctx, stale))
	assertMappings()
	current, err := repo.GetByID(ctx, accounts[0].ID)
	require.NoError(t, err)
	require.Equal(t, "rotated-private", current.Credentials["api_key"])
	peer, err := repo.GetByID(ctx, accounts[1].ID)
	require.NoError(t, err)
	require.Equal(t, "rotated-peer", peer.Credentials["api_key"])
}

func TestAccountConfigGroupCreateMappingConflictIsAtomicAndRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	repo, parent, accounts := accountConfigGroupFixture(t)
	group := testAccountConfigGroup(parent.ID, accounts[0].ID, accounts[1].ID)
	group.ConfirmModelMappingOverwrite = false
	require.ErrorIs(t, repo.SaveAccountConfigGroup(ctx, group), service.ErrAccountConfigGroupMappingConflict)
	require.Zero(t, group.ID)
	groups, err := repo.ListAccountConfigGroups(ctx, parent.ID)
	require.NoError(t, err)
	require.Empty(t, groups)
	for _, account := range accounts[:2] {
		membership, err := repo.GetAccountConfigGroupByAccount(ctx, account.ID)
		require.NoError(t, err)
		require.Nil(t, membership)
		current, err := repo.GetByID(ctx, account.ID)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"old": "old"}, current.Credentials["model_mapping"])
		require.Equal(t, account.Credentials["api_key"], current.Credentials["api_key"])
		require.Equal(t, account.Concurrency, current.Concurrency)
	}
	group.ConfirmModelMappingOverwrite = true
	require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
	require.False(t, group.ConfirmModelMappingOverwrite, "confirmation must be consumed after this save")
	for _, account := range accounts[:2] {
		current, err := repo.GetByID(ctx, account.ID)
		require.NoError(t, err)
		require.Equal(t, group.Config.Credentials["model_mapping"], current.Credentials["model_mapping"])
		require.Equal(t, account.Credentials["api_key"], current.Credentials["api_key"])
	}
	persisted, err := repo.GetAccountConfigGroup(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, persisted.ConfirmModelMappingOverwrite)
}

func TestAccountConfigGroupJoiningMappingConflictPreservesMembersAndConfig(t *testing.T) {
	for _, field := range []string{"model_mapping", "compact_model_mapping"} {
		for _, kind := range []string{"missing alias", "changed target", "empty group mapping"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				ctx := context.Background()
				repo, parent, accounts := accountConfigGroupFixture(t)
				group := testAccountConfigGroup(parent.ID, accounts[0].ID)
				group.ConfirmModelMappingOverwrite = false
				group.Config.Credentials["model_mapping"] = map[string]any{"old": "old"}
				group.Config.Credentials[field] = map[string]any{"old": "old"}
				if kind == "empty group mapping" {
					// Existing members may deliberately clear mappings in settings.
					group.ConfirmModelMappingOverwrite = true
					delete(group.Config.Credentials, field)
				}
				require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
				joiningMapping := map[string]any{"old": "old", "alias": "upstream"}
				if kind == "changed target" {
					joiningMapping = map[string]any{"old": "member-specific-upstream"}
				}
				accounts[1].Credentials[field] = joiningMapping
				require.NoError(t, repo.Update(ctx, accounts[1]))
				before, err := repo.GetAccountConfigGroup(ctx, group.ID)
				require.NoError(t, err)
				group.AccountIDs = append(group.AccountIDs, accounts[1].ID)
				group.Name = "membership plus settings update"
				group.Config.Priority++
				require.ErrorIs(t, repo.SaveAccountConfigGroup(ctx, group), service.ErrAccountConfigGroupMappingConflict)
				after, err := repo.GetAccountConfigGroup(ctx, group.ID)
				require.NoError(t, err)
				require.Equal(t, before, after, "failed membership changes must not save group settings either")
				current, err := repo.GetByID(ctx, accounts[1].ID)
				require.NoError(t, err)
				require.Equal(t, joiningMapping, current.Credentials[field])
				membership, err := repo.GetAccountConfigGroupByAccount(ctx, accounts[1].ID)
				require.NoError(t, err)
				require.Nil(t, membership)

				group.ConfirmModelMappingOverwrite = true
				require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
				require.False(t, group.ConfirmModelMappingOverwrite)
				current, err = repo.GetByID(ctx, accounts[1].ID)
				require.NoError(t, err)
				require.Equal(t, group.Config.Credentials[field], current.Credentials[field])
				require.Equal(t, accounts[1].Credentials["api_key"], current.Credentials["api_key"])

				// Explicitly editing settings for existing members retains its
				// full replacement semantics, including intentional removal.
				group.Config.Credentials = map[string]any{}
				require.NoError(t, repo.SaveAccountConfigGroup(ctx, group))
				for _, id := range group.AccountIDs {
					current, err := repo.GetByID(ctx, id)
					require.NoError(t, err)
					require.NotContains(t, current.Credentials, field)
				}
				// The earlier confirmation must not leak into another join.
				group.AccountIDs = append(group.AccountIDs, accounts[2].ID)
				require.ErrorIs(t, repo.SaveAccountConfigGroup(ctx, group), service.ErrAccountConfigGroupMappingConflict)
			})
		}
	}
}
