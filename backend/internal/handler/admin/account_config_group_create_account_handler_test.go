//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type configGroupCreateHandlerService struct {
	accountConfigGroupHandlerService
	input   *service.CreateAccountInConfigGroupInput
	account *service.Account
}

func (s *configGroupCreateHandlerService) CreateAccountInConfigGroup(_ context.Context, id int64, input *service.CreateAccountInConfigGroupInput) (*service.Account, error) {
	s.calls++
	s.id = id
	s.input = input
	return s.account, s.err
}

func configGroupCreateHandlerRouter(svc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &AccountHandler{adminService: svc}
	router.POST("/account-groups/:id/accounts", handler.CreateAccountInConfigGroup)
	return router
}

func TestAccountConfigGroupCreateAccountHandlerRedactsKey(t *testing.T) {
	groupID := int64(7)
	svc := &configGroupCreateHandlerService{account: &service.Account{
		ID: 99, Name: "group-7-new", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		AccountConfigGroupID: &groupID, AccountConfigGroupName: "group",
		Credentials: map[string]any{"api_key": "test-unique-private-key", "base_url": "https://private.example"},
	}}
	response := accountConfigGroupHandlerRequest(configGroupCreateHandlerRouter(svc), http.MethodPost, "/account-groups/7/accounts", `{"base_url":"https://private.example","api_key":"test-unique-private-key"}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(7), svc.id)
	require.Equal(t, "test-unique-private-key", svc.input.APIKey)
	require.NotContains(t, response.Body.String(), "test-unique-private-key")
	var body struct {
		Data struct {
			ID      int64 `json:"id"`
			GroupID int64 `json:"account_config_group_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, int64(99), body.Data.ID)
	require.Equal(t, groupID, body.Data.GroupID)
}

func TestAccountConfigGroupCreateAccountHandlerValidation(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"/account-groups/0/accounts", `{"api_key":"key"}`},
		{"/account-groups/not-id/accounts", `{"api_key":"key"}`},
		{"/account-groups/7/accounts", `{"api_key": {"secret":"private-key"}}`},
		{"/account-groups/7/accounts", `{"api_key":"private-key"`},
	} {
		svc := &configGroupCreateHandlerService{}
		response := accountConfigGroupHandlerRequest(configGroupCreateHandlerRouter(svc), http.MethodPost, tc.path, tc.body)
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Zero(t, svc.calls)
		require.NotContains(t, response.Body.String(), "private-key")
	}
	svc := &configGroupCreateHandlerService{accountConfigGroupHandlerService: accountConfigGroupHandlerService{err: service.ErrAccountConfigGroupNotFound}}
	response := accountConfigGroupHandlerRequest(configGroupCreateHandlerRouter(svc), http.MethodPost, "/account-groups/7/accounts", `{"api_key":"key"}`)
	require.Equal(t, http.StatusNotFound, response.Code)
	response = accountConfigGroupHandlerRequest(configGroupCreateHandlerRouter(&accountConfigGroupHandlerService{}), http.MethodPost, "/account-groups/7/accounts", `{"api_key":"key"}`)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}

func TestAccountConfigGroupCreateAccountHandlerReplaysAndScopesIdempotency(t *testing.T) {
	previous := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	svc := &configGroupCreateHandlerService{account: &service.Account{ID: 99, Name: "member", Credentials: map[string]any{"api_key": "private-key"}}}
	router := configGroupCreateHandlerRouter(svc)
	call := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "new-group-member-test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	first := call("/account-groups/7/accounts", `{"api_key":"private-key"}`)
	second := call("/account-groups/7/accounts", `{"api_key":"private-key"}`)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, svc.calls)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
	require.NotContains(t, second.Body.String(), "private-key")
	changedGroup := call("/account-groups/8/accounts", `{"api_key":"private-key"}`)
	require.Equal(t, http.StatusConflict, changedGroup.Code)
	changedKey := call("/account-groups/7/accounts", `{"api_key":"changed-private-key"}`)
	require.Equal(t, http.StatusConflict, changedKey.Code)
	require.Equal(t, 1, svc.calls)
}
