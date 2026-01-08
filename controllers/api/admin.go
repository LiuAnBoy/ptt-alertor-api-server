package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Ptt-Alertor/ptt-alertor/jobs"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/julienschmidt/httprouter"
)

// UpdateUserRequest represents an admin user update request
type UpdateUserRequest struct {
	Role    string `json:"role"`
	Enabled bool   `json:"enabled"`
}

// AdminListUsers returns all users (admin only)
func AdminListUsers(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	accounts, err := accountRepo.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "取得用戶列表失敗"})
		return
	}

	if accounts == nil {
		accounts = []*account.Account{}
	}

	writeJSON(w, http.StatusOK, accounts)
}

// AdminGetUser returns a single user (admin only)
func AdminGetUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的用戶 ID"})
		return
	}

	acc, err := accountRepo.FindByID(id)
	if err != nil {
		if err == account.ErrAccountNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "找不到用戶"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "取得用戶失敗"})
		return
	}

	// Get user's subscriptions
	subs, err := subscriptionRepo.ListByUserID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "取得訂閱失敗"})
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
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的用戶 ID"})
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的請求內容"})
		return
	}

	// Validate role
	validRoles := map[string]bool{"admin": true, "user": true}
	if !validRoles[req.Role] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的角色，必須是 admin 或 user"})
		return
	}

	if err := accountRepo.Update(id, req.Role, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "更新用戶失敗"})
		return
	}

	writeJSON(w, http.StatusOK, MessageResponse{Message: "用戶已更新"})
}

// AdminDeleteUser deletes a user (admin only)
func AdminDeleteUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的用戶 ID"})
		return
	}

	// Get account for Redis cleanup before deleting
	acc, _ := accountRepo.FindByID(id)

	if err := accountRepo.Delete(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "刪除用戶失敗"})
		return
	}

	// Sync to Redis (remove user data)
	if acc != nil {
		go redisSync.SyncUserDelete(acc)
	}

	writeJSON(w, http.StatusOK, MessageResponse{Message: "用戶已刪除"})
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
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的請求內容"})
		return
	}

	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "訊息內容不能為空"})
		return
	}

	if len(req.Platforms) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "請指定至少一個平台"})
		return
	}

	bc := &jobs.Broadcaster{}
	bc.Msg = req.Content
	if err := bc.Send(req.Platforms); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, MessageResponse{Message: "廣播已發送"})
}
