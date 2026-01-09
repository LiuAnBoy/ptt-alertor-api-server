package api

import (
	"encoding/json"
	"net/http"

	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/julienschmidt/httprouter"
)

var roleLimitRepo = &account.RoleLimitPostgres{}

// CreateRoleRequest represents a request to create a role
type CreateRoleRequest struct {
	Role             string `json:"role"`
	MaxSubscriptions int    `json:"max_subscriptions"`
	Description      string `json:"description"`
}

// UpdateRoleRequest represents a request to update a role
type UpdateRoleRequest struct {
	MaxSubscriptions int    `json:"max_subscriptions"`
	Description      string `json:"description"`
}

// RoleResponse represents a role with user count
type RoleResponse struct {
	*account.RoleLimit
	UserCount int `json:"user_count"`
}

// AdminListRoles returns all role limits (admin only)
func AdminListRoles(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	roles, err := roleLimitRepo.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得角色列表失敗"})
		return
	}

	// Get user count for each role
	var response []RoleResponse
	for _, role := range roles {
		count, _ := roleLimitRepo.CountUsersByRole(role.Role)
		response = append(response, RoleResponse{
			RoleLimit: role,
			UserCount: count,
		})
	}

	if response == nil {
		response = []RoleResponse{}
	}

	writeJSON(w, http.StatusOK, response)
}

// AdminGetRole returns a single role limit (admin only)
func AdminGetRole(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	roleName := ps.ByName("role")

	role, err := roleLimitRepo.FindByRole(roleName)
	if err != nil {
		if err == account.ErrRoleLimitNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到角色"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得角色失敗"})
		return
	}

	count, _ := roleLimitRepo.CountUsersByRole(roleName)

	writeJSON(w, http.StatusOK, RoleResponse{
		RoleLimit: role,
		UserCount: count,
	})
}

// AdminCreateRole creates a new role limit (admin only)
func AdminCreateRole(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Validate role name
	if req.Role == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "角色名稱為必填"})
		return
	}

	// Check if role already exists
	_, err := roleLimitRepo.FindByRole(req.Role)
	if err == nil {
		writeJSON(w, http.StatusConflict, ErrorResponse{Success: false, Message: "角色已存在"})
		return
	}

	role, err := roleLimitRepo.Create(req.Role, req.MaxSubscriptions, req.Description)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "建立角色失敗"})
		return
	}

	writeJSON(w, http.StatusCreated, role)
}

// AdminUpdateRole updates a role limit (admin only)
func AdminUpdateRole(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	roleName := ps.ByName("role")

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	role, err := roleLimitRepo.Update(roleName, req.MaxSubscriptions, req.Description)
	if err != nil {
		if err == account.ErrRoleLimitNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到角色"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "更新角色失敗"})
		return
	}

	writeJSON(w, http.StatusOK, role)
}

// AdminDeleteRole deletes a role limit (admin only)
func AdminDeleteRole(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	roleName := ps.ByName("role")

	// Prevent deleting built-in roles
	builtInRoles := map[string]bool{"admin": true, "user": true}
	if builtInRoles[roleName] {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "無法刪除內建角色"})
		return
	}

	err := roleLimitRepo.Delete(roleName)
	if err != nil {
		if err == account.ErrRoleLimitNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到角色"})
			return
		}
		if err == account.ErrRoleInUse {
			writeJSON(w, http.StatusConflict, ErrorResponse{Success: false, Message: "角色正在被使用，無法刪除"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "刪除角色失敗"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "刪除成功"})
}
