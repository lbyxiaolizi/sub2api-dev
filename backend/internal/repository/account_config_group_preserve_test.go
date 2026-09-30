package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func expectNoAccountConfigGroup(mock sqlmock.Sqlmock, id int64) {
	mock.ExpectQuery("SELECT proxy_id FROM accounts").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"proxy_id"}).AddRow(nil))
	mock.ExpectQuery("SELECT cg.platform, cg.type, cg.config").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "config"}))
}

func TestPreserveAccountConfigGroupKeepsIdentityAndRuntime(t *testing.T) {
	account := &service.Account{Status: service.StatusError, Schedulable: false,
		Credentials: map[string]any{"api_key": "private", "base_url": "old", "model_mapping": map[string]any{"old": "old"}},
		Extra:       map[string]any{"quota_daily_used": 4.0, "quota_daily_limit": 1.0}}
	group := &service.AccountConfigGroup{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Config: service.AccountConfigGroupConfig{Status: service.StatusActive, Schedulable: true, Concurrency: 12,
			Credentials: map[string]any{"base_url": "new"}, Extra: map[string]any{"quota_daily_limit": 30.0}}}
	preserveAccountConfigGroup(account, group, nil)
	require.Equal(t, "private", account.Credentials["api_key"])
	require.Equal(t, "new", account.Credentials["base_url"])
	require.NotContains(t, account.Credentials, "model_mapping")
	require.Equal(t, 4.0, account.Extra["quota_daily_used"])
	require.Equal(t, 30.0, account.Extra["quota_daily_limit"])
	require.Equal(t, 12, account.Concurrency)
	require.Equal(t, service.StatusError, account.Status)
	require.False(t, account.Schedulable)
}

func TestPreserveAccountConfigGroupPreservesPoolAssignment(t *testing.T) {
	poolID, proxyID := int64(8), int64(9)
	group := &service.AccountConfigGroup{Config: service.AccountConfigGroupConfig{PoolID: &poolID, Status: "inactive", Schedulable: false}}
	account := &service.Account{Status: service.StatusActive, Schedulable: true}
	preserveAccountConfigGroup(account, group, &proxyID)
	require.Equal(t, &proxyID, account.ProxyID)
	require.Equal(t, &poolID, account.PoolID)
	require.Equal(t, "inactive", account.Status)
	require.False(t, account.Schedulable)
}
