package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type configGroupCreateAccountsStub struct {
	*configGroupAccountsStub
	created *Account
	group   *AccountConfigGroup
	err     error
}

func (r *configGroupCreateAccountsStub) CreateAccountInConfigGroup(_ context.Context, group *AccountConfigGroup, account *Account) error {
	if r.err != nil {
		return r.err
	}
	r.created, r.group = account, group
	account.ID = 55
	account.GroupIDs = []int64{group.GroupID}
	return nil
}

func configGroupCreateTestService(t *testing.T) (*adminServiceImpl, *configGroupCreateAccountsStub, *AccountConfigGroup) {
	t.Helper()
	svc, base := configGroupTestService()
	base.accounts[1].Credentials["model_mapping"] = map[string]any{"alias": "actual"}
	base.accounts[1].Credentials["email"] = "private@example.test"
	base.accounts[1].Extra = map[string]any{"quota_daily_used": 42.0, "quota_daily_limit": 100.0}
	group, err := svc.CreateAccountConfigGroup(context.Background(), &CreateAccountConfigGroupInput{Name: "team", GroupID: 7, AccountIDs: []int64{1}})
	require.NoError(t, err)
	repo := &configGroupCreateAccountsStub{configGroupAccountsStub: base}
	svc.accountRepo = repo
	return svc, repo, group
}

func TestCreateAccountInConfigGroupDefaultsAndIndependentKey(t *testing.T) {
	for _, address := range []string{"", "https://example.test/"} {
		t.Run(address, func(t *testing.T) {
			svc, repo, group := configGroupCreateTestService(t)
			created, err := svc.CreateAccountInConfigGroup(context.Background(), group.ID, &CreateAccountInConfigGroupInput{BaseURL: address, APIKey: " new-key "})
			require.NoError(t, err)
			require.Equal(t, []int64{7}, created.GroupIDs)
			require.Equal(t, group.ID, *created.AccountConfigGroupID)
			require.Equal(t, group.Type, created.Type)
			require.Equal(t, group.Platform, created.Platform)
			require.Equal(t, "new-key", created.Credentials["api_key"])
			require.Equal(t, "https://example.test", created.Credentials["base_url"])
			require.NotContains(t, created.Credentials, AccountConfigGroupBaseURLOverrideKey)
			require.NotContains(t, created.Credentials, "email")
			require.NotContains(t, created.Extra, "quota_daily_used")
			require.Equal(t, map[string]any{"alias": "actual"}, created.Credentials["model_mapping"])
			require.Equal(t, "private", repo.accounts[1].Credentials["api_key"])
			require.NotContains(t, group.Config.Credentials, "api_key")
			require.NotEmpty(t, created.Name)
		})
	}
}

func TestCreateAccountInConfigGroupCustomEndpointSurvivesSettingsMerge(t *testing.T) {
	svc, _, group := configGroupCreateTestService(t)
	created, err := svc.CreateAccountInConfigGroup(context.Background(), group.ID, &CreateAccountInConfigGroupInput{BaseURL: " https://private.example/v1/ ", APIKey: "new-key"})
	require.NoError(t, err)
	require.Equal(t, "https://private.example/v1", created.Credentials[AccountConfigGroupBaseURLOverrideKey])
	merged := MergeAccountConfigGroupSettings(created.Credentials, map[string]any{
		"base_url": "https://changed-group.example", "api_base_urls": map[string]any{APIProtocolAnthropic: "https://group-anthropic.example"},
		"model_mapping": map[string]any{"new-alias": "new-model"},
	}, true)
	require.Equal(t, "https://private.example/v1", merged["base_url"])
	require.Equal(t, "https://private.example", merged["api_base_urls"].(map[string]any)[APIProtocolAnthropic])
	require.Equal(t, map[string]any{"new-alias": "new-model"}, merged["model_mapping"])
	require.Equal(t, "new-key", merged["api_key"])
	require.Equal(t, "https://example.test", group.Config.Credentials["base_url"])
}

func TestCreateAccountInConfigGroupRejectsInvalidInputWithoutWrites(t *testing.T) {
	for _, input := range []*CreateAccountInConfigGroupInput{
		nil, {}, {APIKey: " "}, {APIKey: "key\nheader"},
		{APIKey: "key", BaseURL: "file:///tmp/key"}, {APIKey: "key", BaseURL: "https://"},
		{APIKey: "key", BaseURL: "https://user:password@example.test"},
		{APIKey: "key", BaseURL: "https://example.test?token=secret"},
		{APIKey: "key", BaseURL: "https://example.test#fragment"},
	} {
		svc, repo, group := configGroupCreateTestService(t)
		_, err := svc.CreateAccountInConfigGroup(context.Background(), group.ID, input)
		require.Error(t, err)
		require.Nil(t, repo.created)
	}
}

func TestCreateAccountInConfigGroupRejectsCredentialTypesAndPropagatesAtomicFailure(t *testing.T) {
	for _, kind := range []string{AccountTypeOAuth, "service_account", "bedrock"} {
		svc, repo, group := configGroupCreateTestService(t)
		repo.groups[group.ID].Type = kind
		_, err := svc.CreateAccountInConfigGroup(context.Background(), group.ID, &CreateAccountInConfigGroupInput{APIKey: "new-key"})
		require.Error(t, err)
		require.Nil(t, repo.created)
	}
	svc, repo, group := configGroupCreateTestService(t)
	repo.err = errors.New("atomic write failed")
	created, err := svc.CreateAccountInConfigGroup(context.Background(), group.ID, &CreateAccountInConfigGroupInput{APIKey: "new-key"})
	require.ErrorIs(t, err, repo.err)
	require.Nil(t, created)
}

func TestAccountConfigGroupDefaultAddressUsesAdaptiveRouting(t *testing.T) {
	group := &AccountConfigGroup{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
		Config: AccountConfigGroupConfig{Credentials: map[string]any{"api_protocol": APIProtocolAdaptive,
			"api_base_urls": map[string]any{APIProtocolChatCompletions: "https://chat.example/v1"}}}}
	require.Equal(t, "https://chat.example/v1", AccountConfigGroupDefaultBaseURL(group))
	delete(group.Config.Credentials, "api_base_urls")
	require.Equal(t, DefaultOpenCodeGoBaseURL, AccountConfigGroupDefaultBaseURL(group))
}

func TestAccountConfigGroupKeyRotationRetainsOverride(t *testing.T) {
	svc, repo, _ := configGroupCreateTestService(t)
	repo.accounts[1].Credentials[AccountConfigGroupBaseURLOverrideKey] = "https://private.example/v1"
	ApplyAccountConfigGroupEndpointOverride(repo.accounts[1].Credentials)
	input := &UpdateAccountInput{Credentials: map[string]any{"api_key": "rotated", AccountConfigGroupBaseURLOverrideKey: "https://untrusted.example"}}
	require.NoError(t, svc.guardAccountConfigGroupUpdate(context.Background(), repo.accounts[1], input))
	require.Equal(t, "rotated", input.Credentials["api_key"])
	require.Equal(t, "https://private.example/v1", input.Credentials["base_url"])
	require.Equal(t, "https://private.example/v1", input.Credentials[AccountConfigGroupBaseURLOverrideKey])
}
