package repository

import (
	"strconv"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupModelMappingLossGuard(t *testing.T) {
	for _, field := range []string{"model_mapping", "compact_model_mapping"} {
		for _, tc := range []struct {
			name    string
			current any
			target  any
			removed string
			changed string
		}{
			{"empty", nil, nil, "0", "0"},
			{"new mappings", nil, map[string]any{"a": "b"}, "0", "0"},
			{"same mappings", map[string]any{"a": "b"}, map[string]any{"a": "b"}, "0", "0"},
			{"superset", map[string]any{"a": "b"}, map[string]any{"a": "b", "c": "d"}, "0", "0"},
			{"string map equivalent", map[string]string{"a": "b"}, map[string]any{"a": "b"}, "0", "0"},
			{"removed field", map[string]any{"a": "b"}, nil, "1", "0"},
			{"empty object", map[string]any{"a": "b"}, map[string]any{}, "1", "0"},
			{"missing alias", map[string]any{"a": "b", "c": "d"}, map[string]any{"a": "b"}, "1", "0"},
			{"retargeted alias", map[string]any{"a": "b"}, map[string]any{"a": "c"}, "0", "1"},
			{"same count different keys", map[string]any{"a": "b"}, map[string]any{"c": "d"}, "1", "0"},
			{"removed and retargeted", map[string]any{"a": "b", "c": "d"}, map[string]any{"a": "c"}, "1", "1"},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				current := map[string]any{field: tc.current, "api_key": "private-member"}
				target := map[string]any{field: tc.target}
				err := validateAccountConfigGroupModelMappings(73, current, target)
				if tc.removed == "0" && tc.changed == "0" {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, service.ErrAccountConfigGroupMappingConflict)
				require.Equal(t, 409, infraerrors.Code(err))
				require.Equal(t, map[string]string{
					"account_id": "73", "field": field,
					"existing_entries": strconv.Itoa(len(accountConfigGroupMappingEntries(tc.current))), "target_entries": strconv.Itoa(len(accountConfigGroupMappingEntries(tc.target))),
					"removed_entries": tc.removed, "changed_entries": tc.changed,
				}, infraerrors.FromError(err).Metadata)
				require.NotContains(t, err.Error(), "private-member")
			})
		}
	}
}
