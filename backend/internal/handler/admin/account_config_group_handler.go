package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) accountConfigGroupManager(c *gin.Context) service.AccountConfigGroupManager {
	manager, ok := h.adminService.(service.AccountConfigGroupManager)
	if !ok {
		response.Error(c, 503, "account group service is not configured")
		return nil
	}
	return manager
}

func accountConfigGroupID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account group ID")
		return 0, false
	}
	return id, true
}

func (h *AccountHandler) ListAccountConfigGroups(c *gin.Context) {
	manager := h.accountConfigGroupManager(c)
	if manager == nil {
		return
	}
	var groupID int64
	if raw := c.Query("group_id"); raw != "" {
		var err error
		groupID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid parent group ID")
			return
		}
	}
	groups, err := manager.ListAccountConfigGroups(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if groups == nil {
		groups = []service.AccountConfigGroup{}
	}
	response.Success(c, groups)
}
func (h *AccountHandler) GetAccountConfigGroup(c *gin.Context) {
	id, ok := accountConfigGroupID(c)
	if !ok {
		return
	}
	manager := h.accountConfigGroupManager(c)
	if manager == nil {
		return
	}
	group, err := manager.GetAccountConfigGroup(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, group)
}
func (h *AccountHandler) CreateAccountConfigGroup(c *gin.Context) {
	manager := h.accountConfigGroupManager(c)
	if manager == nil {
		return
	}
	var input service.CreateAccountConfigGroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	group, err := manager.CreateAccountConfigGroup(c.Request.Context(), &input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, group)
}
func (h *AccountHandler) UpdateAccountConfigGroup(c *gin.Context) {
	id, ok := accountConfigGroupID(c)
	if !ok {
		return
	}
	manager := h.accountConfigGroupManager(c)
	if manager == nil {
		return
	}
	var input service.UpdateAccountConfigGroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	group, err := manager.UpdateAccountConfigGroup(c.Request.Context(), id, &input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, group)
}
func (h *AccountHandler) DeleteAccountConfigGroup(c *gin.Context) {
	id, ok := accountConfigGroupID(c)
	if !ok {
		return
	}
	manager := h.accountConfigGroupManager(c)
	if manager == nil {
		return
	}
	if err := manager.DeleteAccountConfigGroup(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Account group deleted"})
}

func (h *AccountHandler) CreateAccountInConfigGroup(c *gin.Context) {
	id, ok := accountConfigGroupID(c)
	if !ok {
		return
	}
	creator, ok := h.adminService.(service.AccountConfigGroupAccountCreator)
	if !ok {
		response.Error(c, 503, "account group account creation is not configured")
		return
	}
	var input service.CreateAccountInConfigGroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// Parsing errors may contain values; never echo an API key from this form.
		response.BadRequest(c, "Invalid account group account request")
		return
	}
	payload := struct {
		GroupID int64 `json:"group_id"`
		service.CreateAccountInConfigGroupInput
	}{GroupID: id, CreateAccountInConfigGroupInput: input}
	executeAdminIdempotentJSON(c, "admin.account_groups.accounts.create", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		account, err := creator.CreateAccountInConfigGroup(ctx, id, &input)
		if err != nil {
			return nil, err
		}
		return h.accountResponseFromService(account), nil
	})
}
