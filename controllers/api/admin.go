package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/jobs"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/models/stats"
	"github.com/julienschmidt/httprouter"
)

var redisSync = &account.RedisSync{}

// UpdateUserRequest represents an admin user update request
type UpdateUserRequest struct {
	Role    string `json:"role"`
	Enabled bool   `json:"enabled"`
}

// AdminListUsers returns users with pagination and search (admin only)
func AdminListUsers(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	q := r.URL.Query()

	// Get search query
	search := q.Get("q")

	// Get pagination params (default: page=1, limit=20)
	page := 1
	limit := 20

	if p := q.Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	result, err := accountRepo.List(search, page, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得用戶列表失敗"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// AdminGetUser returns a single user (admin only)
func AdminGetUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的用戶 ID"})
		return
	}

	acc, err := accountRepo.FindByID(id)
	if err != nil {
		if err == account.ErrAccountNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到用戶"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得用戶失敗"})
		return
	}

	// Get user's subscriptions
	subs, err := subscriptionRepo.ListByUserID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱失敗"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"account":       acc,
		"subscriptions": subs,
	})
}

// AdminUpdateUser updates a user (admin only)
func AdminUpdateUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的用戶 ID"})
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Validate role
	validRoles := map[string]bool{"admin": true, "user": true}
	if !validRoles[req.Role] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的角色，必須是 admin 或 user"})
		return
	}

	if err := accountRepo.Update(id, req.Role, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "更新用戶失敗"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "用戶已更新"})
}

// AdminDeleteUser deletes a user (admin only)
func AdminDeleteUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的用戶 ID"})
		return
	}

	// Get account for Redis cleanup before deleting
	acc, _ := accountRepo.FindByID(id)

	if err := accountRepo.Delete(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "刪除用戶失敗"})
		return
	}

	// Sync to Redis (remove user data)
	if acc != nil {
		go redisSync.SyncUserDelete(acc)
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "用戶已刪除"})
}

// BroadcastRequest represents a broadcast request
type BroadcastRequest struct {
	Platforms []string `json:"platforms"`
	Content   string   `json:"content"`
}

// AdminBroadcast sends a message to all users (admin only)
func AdminBroadcast(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req BroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "訊息內容不能為空"})
		return
	}

	if len(req.Platforms) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "請指定至少一個平台"})
		return
	}

	bc := &jobs.Broadcaster{}
	bc.Msg = req.Content
	if err := bc.Send(req.Platforms); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "廣播已發送"})
}

// AdminLogin handles admin login (requires admin role)
func AdminLogin(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Find account
	acc, err := accountRepo.FindByEmail(req.Email)
	if err != nil {
		if err == account.ErrAccountNotFound {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "電子郵件或密碼錯誤"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "查詢帳號失敗"})
		return
	}

	// Check if account is enabled
	if !acc.Enabled {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "帳號已停用"})
		return
	}

	// Check if account is admin
	if acc.Role != "admin" {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "權限不足"})
		return
	}

	// Check password
	if !auth.CheckPassword(req.Password, acc.Password) {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "電子郵件或密碼錯誤"})
		return
	}

	// Generate token
	token, err := auth.GenerateToken(acc.ID, acc.Email, acc.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "產生令牌失敗"})
		return
	}

	writeJSON(w, http.StatusOK, TokenResponse{Token: token})
}

// AdminInit returns admin dashboard statistics
func AdminInit(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	response, err := stats.GetAdminStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得統計資料失敗"})
		return
	}

	writeJSON(w, http.StatusOK, response)
}
