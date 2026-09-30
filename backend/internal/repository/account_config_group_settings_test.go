package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupPlanOverrideReplacement(t *testing.T) {
	for _, tc := range []struct {
		name        string
		previous    map[string]any
		replacement map[string]any
		want        any
	}{
		{"automatic remains independent", nil, map[string]any{}, "team"},
		{"set override", nil, map[string]any{"plan_type": "pro"}, "pro"},
		{"change override", map[string]any{"plan_type": "plus"}, map[string]any{"plan_type": "pro"}, "pro"},
		{"clear override", map[string]any{"plan_type": "pro"}, map[string]any{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := map[string]any{"access_token": "independent", "plan_type": "team", "base_url": "https://old.example"}
			result := replaceAccountConfigGroupCredentials(original, tc.previous, tc.replacement)
			require.Equal(t, "independent", result["access_token"])
			require.NotContains(t, result, "base_url")
			if tc.want == nil {
				require.NotContains(t, result, "plan_type")
			} else {
				require.Equal(t, tc.want, result["plan_type"])
			}
			require.Equal(t, "team", original["plan_type"], "do not mutate the member snapshot")
		})
	}
}

func TestAccountConfigGroupQuotaResetDerivationKeepsMemberUsage(t *testing.T) {
	now := time.Now().UTC()
	settings := map[string]any{
		"quota_daily_limit": 100.0, "quota_daily_reset_mode": "fixed", "quota_daily_reset_hour": 0.0,
		"quota_weekly_limit": 500.0, "quota_weekly_reset_mode": "fixed", "quota_weekly_reset_day": 1.0,
		"quota_weekly_reset_hour": 0.0, "quota_reset_timezone": "UTC",
		"quota_notify_daily_threshold_type": "percentage",
	}
	for _, usage := range []float64{12, 37} {
		original := map[string]any{
			"quota_daily_used": usage, "quota_weekly_used": usage * 2,
			"quota_daily_start": now.Format(time.RFC3339), "quota_weekly_start": now.Format(time.RFC3339),
			"quota_daily_reset_at": "2000-01-01T00:00:00Z", "quota_weekly_reset_at": "2000-01-01T00:00:00Z",
			"codex_fingerprint_seed": "member-specific", "upstream_billing_probe": map[string]any{"balance": usage},
		}
		result := service.PrepareAccountConfigGroupMemberExtra(&service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Extra: original}, settings)
		require.Equal(t, usage, result["quota_daily_used"])
		require.Equal(t, usage*2, result["quota_weekly_used"])
		require.Equal(t, "member-specific", result["codex_fingerprint_seed"])
		require.Equal(t, original["upstream_billing_probe"], result["upstream_billing_probe"])
		require.Equal(t, "percentage", result["quota_notify_daily_threshold_type"])
		for _, dimension := range []string{"daily", "weekly"} {
			resetAt, err := time.Parse(time.RFC3339, result["quota_"+dimension+"_reset_at"].(string))
			require.NoError(t, err)
			require.True(t, resetAt.After(now))
		}
		require.NotContains(t, settings, "quota_daily_reset_at", "derived state must not enter the shared config")
		require.Equal(t, "2000-01-01T00:00:00Z", original["quota_daily_reset_at"])

		rolling := service.PrepareAccountConfigGroupMemberExtra(&service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Extra: result}, map[string]any{"quota_daily_limit": 100.0, "quota_weekly_limit": 500.0})
		require.NotContains(t, rolling, "quota_daily_reset_at")
		require.NotContains(t, rolling, "quota_weekly_reset_at")
		require.Equal(t, usage, rolling["quota_daily_used"])
		require.Equal(t, usage*2, rolling["quota_weekly_used"])
	}
}

func TestPreserveAccountConfigGroupRetainsManualTierDuringRefresh(t *testing.T) {
	account := &service.Account{Credentials: map[string]any{"access_token": "new-token", "plan_type": "plus"}}
	group := &service.AccountConfigGroup{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Config: service.AccountConfigGroupConfig{Credentials: map[string]any{"plan_type": "pro"}}}
	preserveAccountConfigGroup(account, group, nil)
	require.Equal(t, "new-token", account.Credentials["access_token"])
	require.Equal(t, "pro", account.Credentials["plan_type"])

	group.Config.Credentials = map[string]any{}
	account.Credentials["plan_type"] = "team"
	preserveAccountConfigGroup(account, group, nil)
	require.Equal(t, "team", account.Credentials["plan_type"])
}
