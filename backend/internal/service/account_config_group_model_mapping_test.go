package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupCopiesSelectedSourceModelMappings(t *testing.T) {
	for _, sourceID := range []int64{0, 2} {
		t.Run(map[int64]string{0: "default source", 2: "explicit source"}[sourceID], func(t *testing.T) {
			svc, repo := configGroupTestService()
			selectedID := sourceID
			if selectedID == 0 {
				selectedID = 1
			}
			mapping := map[string]any{"client-model": "upstream-model", "client-*": "upstream-*"}
			compactMapping := map[string]any{"compact-client": "compact-upstream"}
			repo.accounts[selectedID].Credentials["model_mapping"] = mapping
			repo.accounts[selectedID].Credentials["compact_model_mapping"] = compactMapping
			group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{
				Name: "mapping source", GroupID: 7, AccountIDs: []int64{1, 2}, SourceAccountID: sourceID,
			})
			require.NoError(t, err)
			require.Equal(t, mapping, group.Config.Credentials["model_mapping"])
			require.Equal(t, compactMapping, group.Config.Credentials["compact_model_mapping"])
			require.NotContains(t, group.Config.Credentials, "api_key")
		})
	}
}

func TestAccountConfigGroupMemberAndUnrelatedConfigUpdatesPreserveModelMappings(t *testing.T) {
	ctx := context.Background()
	svc, repo := configGroupTestService()
	mapping := map[string]any{"client-model": "upstream-model"}
	compactMapping := map[string]any{"compact-client": "compact-upstream"}
	repo.accounts[1].Credentials["model_mapping"] = mapping
	repo.accounts[1].Credentials["compact_model_mapping"] = compactMapping
	group, err := svc.CreateAccountConfigGroup(ctx, &CreateAccountConfigGroupInput{
		Name: "mapping source", GroupID: 7, AccountIDs: []int64{1},
	})
	require.NoError(t, err)
	ids := []int64{1, 2, 3}
	group, err = svc.UpdateAccountConfigGroup(ctx, group.ID, &UpdateAccountConfigGroupInput{AccountIDs: &ids})
	require.NoError(t, err)
	require.Equal(t, mapping, group.Config.Credentials["model_mapping"])
	require.Equal(t, compactMapping, group.Config.Credentials["compact_model_mapping"])
	group, err = svc.UpdateAccountConfigGroup(ctx, group.ID, &UpdateAccountConfigGroupInput{
		Config: json.RawMessage(`{"concurrency":7,"extra":{"quota_daily_limit":100}}`),
	})
	require.NoError(t, err)
	require.Equal(t, mapping, group.Config.Credentials["model_mapping"])
	require.Equal(t, compactMapping, group.Config.Credentials["compact_model_mapping"])
}

func TestAccountConfigGroupCredentialRefreshGuardPreservesModelMappings(t *testing.T) {
	ctx := context.Background()
	svc, repo := configGroupTestService()
	mapping := map[string]any{"client-model": "upstream-model"}
	compactMapping := map[string]any{"compact-client": "compact-upstream"}
	account := repo.accounts[1]
	account.Credentials["model_mapping"] = mapping
	account.Credentials["compact_model_mapping"] = compactMapping
	_, err := svc.CreateAccountConfigGroup(ctx, &CreateAccountConfigGroupInput{
		Name: "refresh mappings", GroupID: 7, AccountIDs: []int64{1},
	})
	require.NoError(t, err)
	refresh := &UpdateAccountInput{Credentials: map[string]any{"api_key": "rotated-private"}}
	require.NoError(t, svc.guardAccountConfigGroupUpdate(ctx, account, refresh))
	require.Equal(t, "rotated-private", refresh.Credentials["api_key"])
	require.Equal(t, mapping, refresh.Credentials["model_mapping"])
	require.Equal(t, compactMapping, refresh.Credentials["compact_model_mapping"])
}
