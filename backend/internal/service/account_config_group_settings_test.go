package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupQuotaNotificationThresholdTypes(t *testing.T) {
	svc, repo := configGroupTestService()
	sourceExtra := map[string]any{"quota_daily_used": 12.0}
	for _, dimension := range []string{"daily", "weekly", "total"} {
		sourceExtra["quota_notify_"+dimension+"_enabled"] = true
		sourceExtra["quota_notify_"+dimension+"_threshold"] = 20.0
		sourceExtra["quota_notify_"+dimension+"_threshold_type"] = "percentage"
	}
	repo.accounts[1].Extra = sourceExtra
	group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{
		Name: "quota settings", GroupID: 7, AccountIDs: []int64{1, 2},
	})
	require.NoError(t, err)
	require.NotContains(t, group.Config.Extra, "quota_daily_used")
	for _, dimension := range []string{"daily", "weekly", "total"} {
		require.Equal(t, "percentage", group.Config.Extra["quota_notify_"+dimension+"_threshold_type"])
	}

	group, err = svc.UpdateAccountConfigGroup(context.Background(), group.ID, &UpdateAccountConfigGroupInput{
		Config: json.RawMessage(`{"extra":{"quota_notify_daily_enabled":true,"quota_notify_daily_threshold":5,"quota_notify_daily_threshold_type":"fixed"}}`),
	})
	require.NoError(t, err)
	require.Equal(t, "fixed", group.Config.Extra["quota_notify_daily_threshold_type"])
	require.NotContains(t, group.Config.Extra, "quota_notify_weekly_enabled")
	require.NotContains(t, group.Config.Extra, "quota_notify_total_threshold_type")
	member := MergeAccountConfigGroupSettings(sourceExtra, group.Config.Extra, false)
	require.Equal(t, 12.0, member["quota_daily_used"])
	require.NotContains(t, member, "quota_notify_weekly_threshold_type")
}

func TestAccountConfigGroupGrokMediaSettingsValidationAndReset(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		want          any
		wantError     bool
	}{
		{"enabled", `{"extra":{"grok_media_eligible":true}}`, true, false},
		{"disabled", `{"extra":{"grok_media_eligible":false}}`, false, false},
		{"automatic", `{"extra":{"grok_media_eligible":null}}`, nil, false},
		{"removed", `{"extra":{}}`, nil, false},
		{"invalid", `{"extra":{"grok_media_eligible":"false"}}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := configGroupTestService()
			for _, account := range repo.accounts {
				account.Platform, account.Type = PlatformGrok, AccountTypeOAuth
				account.Extra = map[string]any{GrokMediaEligibleExtraKey: true}
			}
			group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{
				Name: "grok settings", GroupID: 7, AccountIDs: []int64{1, 2},
			})
			require.NoError(t, err)
			updated, err := svc.UpdateAccountConfigGroup(context.Background(), group.ID, &UpdateAccountConfigGroupInput{Config: json.RawMessage(tc.payload)})
			if tc.wantError {
				require.Error(t, err)
				require.Equal(t, 1, repo.saves)
				return
			}
			require.NoError(t, err)
			if tc.want == nil {
				require.NotContains(t, updated.Config.Extra, GrokMediaEligibleExtraKey)
			} else {
				require.Equal(t, tc.want, updated.Config.Extra[GrokMediaEligibleExtraKey])
			}
		})
	}
}

func TestAccountConfigGroupManualPlanOverrideAndAutomaticDetection(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		want          string
		wantError     bool
	}{
		{"manual override", `{"credentials":{"plan_type":" pro "}}`, "pro", false},
		{"automatic empty", `{"credentials":{"plan_type":""}}`, "", false},
		{"automatic null", `{"credentials":{"plan_type":null}}`, "", false},
		{"automatic absent", `{"credentials":{}}`, "", false},
		{"invalid", `{"credentials":{"plan_type":true}}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := configGroupTestService()
			for _, account := range repo.accounts {
				account.Type = AccountTypeOAuth
				account.Credentials = map[string]any{"access_token": "member-token", "plan_type": "plus"}
			}
			group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{
				Name: "oauth settings", GroupID: 7, AccountIDs: []int64{1, 2},
			})
			require.NoError(t, err)
			require.Equal(t, "plus", group.Config.Credentials["plan_type"])
			updated, err := svc.UpdateAccountConfigGroup(context.Background(), group.ID, &UpdateAccountConfigGroupInput{Config: json.RawMessage(tc.payload)})
			if tc.wantError {
				require.Error(t, err)
				require.Equal(t, 1, repo.saves)
				return
			}
			require.NoError(t, err)
			if tc.want == "" {
				require.NotContains(t, updated.Config.Credentials, "plan_type")
			} else {
				require.Equal(t, tc.want, updated.Config.Credentials["plan_type"])
			}
			refreshed := MergeAccountConfigGroupSettings(map[string]any{
				"access_token": "refreshed-member-token", "plan_type": "team", "chatgpt_account_id": "member-workspace",
			}, updated.Config.Credentials, true)
			require.Equal(t, "refreshed-member-token", refreshed["access_token"])
			require.Equal(t, "member-workspace", refreshed["chatgpt_account_id"])
			if tc.want == "" {
				require.Equal(t, "team", refreshed["plan_type"])
			} else {
				require.Equal(t, tc.want, refreshed["plan_type"])
			}
		})
	}
}

func TestAccountConfigGroupEmptyMapsRemoveSettingsAndKeepIdentity(t *testing.T) {
	svc, _ := configGroupTestService()
	group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{
		Name: "clear settings", GroupID: 7, AccountIDs: []int64{1, 2},
	})
	require.NoError(t, err)
	group, err = svc.UpdateAccountConfigGroup(context.Background(), group.ID, &UpdateAccountConfigGroupInput{
		Config: json.RawMessage(`{"credentials":{},"extra":{},"expires_at":0,"proxy_id":0,"pool_id":0,"load_factor":0}`),
	})
	require.NoError(t, err)
	require.Empty(t, group.Config.Credentials)
	require.Empty(t, group.Config.Extra)
	require.Nil(t, group.Config.ExpiresAt)
	require.Nil(t, group.Config.ProxyID)
	require.Nil(t, group.Config.PoolID)
	require.Nil(t, group.Config.LoadFactor)
	member := MergeAccountConfigGroupSettings(map[string]any{
		"api_key": "member-secret", "base_url": "https://old.example", "temp_unschedulable_enabled": true,
	}, group.Config.Credentials, true)
	require.Equal(t, map[string]any{"api_key": "member-secret"}, member)
}

func TestAccountConfigGroupFingerprintSettingGeneratesIndependentSeeds(t *testing.T) {
	settings := map[string]any{"codex_fingerprint_mode": "full"}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	first := PrepareAccountConfigGroupMemberExtra(account, settings)
	second := PrepareAccountConfigGroupMemberExtra(account, settings)
	firstSeed, firstOK := codexFingerprintSeed(first)
	secondSeed, secondOK := codexFingerprintSeed(second)
	require.True(t, firstOK)
	require.True(t, secondOK)
	require.NotEqual(t, firstSeed, secondSeed)
	require.NotContains(t, settings, codexFingerprintSeedExtraKey)

	existingSeed := uuid.NewString()
	account.Extra = map[string]any{codexFingerprintSeedExtraKey: existingSeed}
	result := PrepareAccountConfigGroupMemberExtra(account, settings)
	require.Equal(t, existingSeed, result[codexFingerprintSeedExtraKey])
	require.Equal(t, "full", result[codexFingerprintModeExtraKey])
}

func TestAccountConfigGroupOAuthRefreshGuardPreservesTierPolicy(t *testing.T) {
	for _, override := range []bool{false, true} {
		svc, repo := configGroupTestService()
		account := repo.accounts[1]
		account.Type = AccountTypeOAuth
		account.Credentials = map[string]any{"access_token": "old-token", "plan_type": "plus"}
		group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{
			Name: "refresh settings", GroupID: 7, AccountIDs: []int64{1},
		})
		require.NoError(t, err)
		if !override {
			repo.groups[group.ID].Config.Credentials = map[string]any{}
		}
		refresh := &UpdateAccountInput{Credentials: map[string]any{"access_token": "new-token", "plan_type": "team"}}
		require.NoError(t, svc.guardAccountConfigGroupUpdate(context.Background(), account, refresh))
		require.Equal(t, "new-token", refresh.Credentials["access_token"])
		if override {
			require.Equal(t, "plus", refresh.Credentials["plan_type"])
		} else {
			require.Equal(t, "team", refresh.Credentials["plan_type"])
		}
		manualEdit := &UpdateAccountInput{Credentials: map[string]any{"plan_type": "pro"}}
		require.ErrorIs(t, svc.guardAccountConfigGroupUpdate(context.Background(), account, manualEdit), ErrAccountConfigGroupManaged)
	}
}

func TestAccountConfigGroupAntigravityOveragesSettingUpdatesLocalLimits(t *testing.T) {
	for _, tc := range []struct {
		name             string
		previous, next   bool
		wantLimits       bool
		wantCreditLimits bool
		wantLegacyState  bool
	}{
		{"enable", false, true, false, false, false},
		{"disable", true, false, true, false, false},
		{"unchanged", true, true, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := map[string]any{creditsExhaustedKey: "credit-reset", "model-a": "model-reset"}
			account := &Account{Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: map[string]any{
				"allow_overages": tc.previous, "antigravity_credits_overages": true,
				modelRateLimitsKey: limits, "provider_usage": 7.0,
			}}
			result := PrepareAccountConfigGroupMemberExtra(account, map[string]any{"allow_overages": tc.next})
			require.Equal(t, tc.next, result["allow_overages"])
			require.Equal(t, 7.0, result["provider_usage"])
			_, hasLimits := result[modelRateLimitsKey]
			require.Equal(t, tc.wantLimits, hasLimits)
			if tc.wantLimits {
				updatedLimits := result[modelRateLimitsKey].(map[string]any)
				_, hasCreditLimits := updatedLimits[creditsExhaustedKey]
				require.Equal(t, tc.wantCreditLimits, hasCreditLimits)
				require.Equal(t, "model-reset", updatedLimits["model-a"])
			}
			_, hasLegacyState := result["antigravity_credits_overages"]
			require.Equal(t, tc.wantLegacyState, hasLegacyState)
			require.Contains(t, limits, creditsExhaustedKey, "do not mutate the input snapshot")
		})
	}
}
