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

type accountConfigGroupHandlerService struct {
	service.AdminService
	group    *service.AccountConfigGroup
	groups   []service.AccountConfigGroup
	err      error
	calls    int
	id       int64
	parentID int64
	create   *service.CreateAccountConfigGroupInput
	update   *service.UpdateAccountConfigGroupInput
}

func (s *accountConfigGroupHandlerService) ListAccountConfigGroups(_ context.Context, parentID int64) ([]service.AccountConfigGroup, error) {
	s.calls++
	s.parentID = parentID
	return s.groups, s.err
}
func (s *accountConfigGroupHandlerService) GetAccountConfigGroup(_ context.Context, id int64) (*service.AccountConfigGroup, error) {
	s.calls++
	s.id = id
	return s.group, s.err
}
func (s *accountConfigGroupHandlerService) CreateAccountConfigGroup(_ context.Context, input *service.CreateAccountConfigGroupInput) (*service.AccountConfigGroup, error) {
	s.calls++
	s.create = input
	return s.group, s.err
}
func (s *accountConfigGroupHandlerService) UpdateAccountConfigGroup(_ context.Context, id int64, input *service.UpdateAccountConfigGroupInput) (*service.AccountConfigGroup, error) {
	s.calls++
	s.id = id
	s.update = input
	return s.group, s.err
}
func (s *accountConfigGroupHandlerService) DeleteAccountConfigGroup(_ context.Context, id int64) error {
	s.calls++
	s.id = id
	return s.err
}

func accountConfigGroupHandlerRouter(svc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &AccountHandler{adminService: svc}
	router.GET("/account-groups", handler.ListAccountConfigGroups)
	router.POST("/account-groups", handler.CreateAccountConfigGroup)
	router.GET("/account-groups/:id", handler.GetAccountConfigGroup)
	router.PUT("/account-groups/:id", handler.UpdateAccountConfigGroup)
	router.DELETE("/account-groups/:id", handler.DeleteAccountConfigGroup)
	return router
}
func accountConfigGroupHandlerRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestAccountConfigGroupHandlerListUsesEmptyArrayAndParentFilter(t *testing.T) {
	svc := &accountConfigGroupHandlerService{}
	router := accountConfigGroupHandlerRouter(svc)
	response := accountConfigGroupHandlerRequest(router, http.MethodGet, "/account-groups?group_id=21", "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(21), svc.parentID)
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.NotNil(t, envelope.Data)
	require.Empty(t, envelope.Data)
}
func TestAccountConfigGroupHandlerRejectsInvalidIdentifiers(t *testing.T) {
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			t.Run(method+id, func(t *testing.T) {
				svc := &accountConfigGroupHandlerService{}
				response := accountConfigGroupHandlerRequest(accountConfigGroupHandlerRouter(svc), method, "/account-groups/"+id, `{}`)
				require.Equal(t, http.StatusBadRequest, response.Code)
				require.Zero(t, svc.calls)
			})
		}
		t.Run("parent"+id, func(t *testing.T) {
			svc := &accountConfigGroupHandlerService{}
			response := accountConfigGroupHandlerRequest(accountConfigGroupHandlerRouter(svc), http.MethodGet, "/account-groups?group_id="+id, "")
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Zero(t, svc.calls)
		})
	}
}
func TestAccountConfigGroupHandlerForwardsCreateAndUpdateSettings(t *testing.T) {
	svc := &accountConfigGroupHandlerService{group: &service.AccountConfigGroup{ID: 8, Name: "pooled", GroupID: 4, AccountIDs: []int64{1, 2}}}
	router := accountConfigGroupHandlerRouter(svc)
	response := accountConfigGroupHandlerRequest(router, http.MethodPost, "/account-groups", `{"name":"pooled","group_id":4,"account_ids":[1,2],"source_account_id":1}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(4), svc.create.GroupID)
	require.Equal(t, []int64{1, 2}, svc.create.AccountIDs)
	require.Equal(t, int64(1), svc.create.SourceAccountID)
	response = accountConfigGroupHandlerRequest(router, http.MethodPut, "/account-groups/8", `{"account_ids":[],"config":{"proxy_id":null,"concurrency":0,"schedulable":false}}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(8), svc.id)
	require.NotNil(t, svc.update.AccountIDs)
	require.Empty(t, *svc.update.AccountIDs)
	require.JSONEq(t, `{"proxy_id":null,"concurrency":0,"schedulable":false}`, string(svc.update.Config))
}
func TestAccountConfigGroupHandlerPropagatesConflictsAndMissingResources(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"membership conflict", service.ErrAccountConfigGroupConflict, http.StatusConflict},
		{"mapping conflict", service.ErrAccountConfigGroupMappingConflict, http.StatusConflict},
		{"stale update", service.ErrAccountConfigGroupStale, http.StatusConflict},
		{"invalid members", service.ErrAccountConfigGroupInvalid, http.StatusBadRequest},
		{"missing group", service.ErrAccountConfigGroupNotFound, http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &accountConfigGroupHandlerService{err: test.err}
			response := accountConfigGroupHandlerRequest(accountConfigGroupHandlerRouter(svc), http.MethodPut, "/account-groups/8", `{"name":"updated"}`)
			require.Equal(t, test.status, response.Code)
			require.Equal(t, 1, svc.calls)
		})
	}
}

func TestAccountConfigGroupHandlerMappingOverwriteConfirmation(t *testing.T) {
	svc := &accountConfigGroupHandlerService{group: &service.AccountConfigGroup{ID: 8, ConfirmModelMappingOverwrite: true}}
	router := accountConfigGroupHandlerRouter(svc)
	for _, body := range []string{
		`{"name":"pooled","group_id":4,"account_ids":[1,2],"confirm_model_mapping_overwrite":true}`,
		`{"account_ids":[1,2],"confirm_model_mapping_overwrite":true}`,
	} {
		method, path := http.MethodPost, "/account-groups"
		if !strings.Contains(body, "group_id") {
			method, path = http.MethodPut, "/account-groups/8"
		}
		response := accountConfigGroupHandlerRequest(router, method, path, body)
		require.Equal(t, http.StatusOK, response.Code)
		if method == http.MethodPost {
			require.True(t, svc.create.ConfirmModelMappingOverwrite)
		} else {
			require.True(t, svc.update.ConfirmModelMappingOverwrite)
		}
		require.NotContains(t, response.Body.String(), "confirm_model_mapping_overwrite", "confirmation is request-only")
	}
	response := accountConfigGroupHandlerRequest(router, http.MethodPut, "/account-groups/8", `{"confirm_model_mapping_overwrite":"true"}`)
	require.Equal(t, http.StatusBadRequest, response.Code)
}
func TestAccountConfigGroupHandlerMalformedJSONDoesNotMutate(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		svc := &accountConfigGroupHandlerService{}
		path := "/account-groups"
		if method == http.MethodPut {
			path += "/8"
		}
		response := accountConfigGroupHandlerRequest(accountConfigGroupHandlerRouter(svc), method, path, `{"config":`)
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Zero(t, svc.calls)
	}
}
