package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupMetadataSurvivesLiteProjection(t *testing.T) {
	id := int64(7)
	full := AccountFromService(&service.Account{
		ID: 19, AccountConfigGroupID: &id, AccountConfigGroupName: "shared settings",
	})
	lite := AccountListItemFromAccount(full)
	for _, value := range []any{full, lite} {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(data, &payload))
		require.Equal(t, float64(7), payload["account_config_group_id"])
		require.Equal(t, "shared settings", payload["account_config_group_name"])
	}
}

func TestUngroupedAccountOmitsConfigGroupMetadata(t *testing.T) {
	account := AccountFromService(&service.Account{ID: 19})
	for _, value := range []any{account, AccountListItemFromAccount(account)} {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(data, &payload))
		require.NotContains(t, payload, "account_config_group_id")
		require.NotContains(t, payload, "account_config_group_name")
	}
}
