package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigGroupCreateAccountOmitsCredentialBodyFromAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "/api/v1/admin/account-groups/:id/accounts"
	require.Contains(t, auditBodyOmittedRoutes, "POST "+route)
	repository := &auditCaptureRepository{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 77})
		c.Set(string(ContextKeyUserRole), "admin")
		c.Next()
	})
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(auditService)))
	router.POST(route, func(c *gin.Context) {
		var body struct {
			APIKey string `json:"api_key"`
		}
		require.NoError(t, c.ShouldBindJSON(&body))
		require.Equal(t, "account-group-api-key-canary", body.APIKey)
		c.JSON(http.StatusOK, gin.H{"id": 123})
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-groups/42/accounts", bytes.NewBufferString(`{"api_key":"account-group-api-key-canary","base_url":"https://example.invalid/private-endpoint-canary"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	auditService.Stop()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	require.Len(t, repository.logs, 1)
	entry := repository.logs[0]
	require.Equal(t, "admin.account_groups.accounts.create", entry.Action)
	require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
	require.NotContains(t, entry.RequestBody, "canary")
}
