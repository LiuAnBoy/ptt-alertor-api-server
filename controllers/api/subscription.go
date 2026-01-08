package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/rss"
	"github.com/julienschmidt/httprouter"
)

var subscriptionRepo = &account.SubscriptionPostgres{}
var redisSync = &account.RedisSync{}

// CreateSubscriptionRequest represents a subscription creation request
type CreateSubscriptionRequest struct {
	Board   string `json:"board"`
	SubType string `json:"sub_type"`
	Value   string `json:"value"`
}

// UpdateSubscriptionRequest represents a subscription update request
type UpdateSubscriptionRequest struct {
	Board   string `json:"board"`
	SubType string `json:"sub_type"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// ListSubscriptions returns all subscriptions for the current user
func ListSubscriptions(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	subs, err := subscriptionRepo.ListByUserID(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱列表失敗"})
		return
	}

	if subs == nil {
		subs = []*account.Subscription{}
	}

	writeJSON(w, http.StatusOK, subs)
}

// CreateSubscription creates a new subscription
func CreateSubscription(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Validate board
	if req.Board == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板為必填"})
		return
	}

	// Check if board exists
	if !rss.CheckBoardExist(req.Board) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板不存在"})
		return
	}

	// Validate sub_type
	validSubTypes := map[string]bool{"keyword": true, "author": true, "pushsum": true}
	if !validSubTypes[req.SubType] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱類型，必須是 keyword、author 或 pushsum"})
		return
	}

	// Validate value
	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "訂閱值為必填"})
		return
	}

	// Check subscription limit for regular users
	if claims.Role == "user" {
		count, err := subscriptionRepo.CountByUserID(claims.UserID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "檢查訂閱數量失敗"})
			return
		}
		if count >= account.MaxSubscriptionsForUser {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "已達訂閱上限，一般用戶最多 3 組訂閱"})
			return
		}
	}

	// Create subscription
	sub, err := subscriptionRepo.Create(claims.UserID, req.Board, req.SubType, req.Value)
	if err != nil {
		if err == account.ErrSubscriptionExists {
			writeJSON(w, http.StatusConflict, ErrorResponse{Success: false, Message: "訂閱已存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "建立訂閱失敗"})
		return
	}

	// Sync to Redis
	acc, _ := accountRepo.FindByID(claims.UserID)
	if acc != nil {
		go redisSync.SyncSubscriptionCreate(sub, acc)
	}

	writeJSON(w, http.StatusCreated, SuccessResponse{Success: true, Message: "訂閱已建立"})
}

// GetSubscription returns a single subscription
func GetSubscription(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱 ID"})
		return
	}

	sub, err := subscriptionRepo.FindByID(id)
	if err != nil {
		if err == account.ErrSubscriptionNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱失敗"})
		return
	}

	// Check ownership
	if sub.UserID != claims.UserID && claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "禁止存取"})
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

// UpdateSubscription updates a subscription
func UpdateSubscription(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱 ID"})
		return
	}

	// Check ownership
	sub, err := subscriptionRepo.FindByID(id)
	if err != nil {
		if err == account.ErrSubscriptionNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱失敗"})
		return
	}

	if sub.UserID != claims.UserID && claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "禁止存取"})
		return
	}

	var req UpdateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Validate board
	if req.Board == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板為必填"})
		return
	}

	// Check if board exists
	if !rss.CheckBoardExist(req.Board) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板不存在"})
		return
	}

	// Validate sub_type
	validSubTypes := map[string]bool{"keyword": true, "author": true, "pushsum": true}
	if !validSubTypes[req.SubType] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱類型，必須是 keyword、author 或 pushsum"})
		return
	}

	// Validate value
	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "訂閱值為必填"})
		return
	}

	if err := subscriptionRepo.Update(id, req.Board, req.SubType, req.Value, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "更新訂閱失敗"})
		return
	}

	// Update sub for Redis sync
	sub.Board = req.Board
	sub.SubType = req.SubType
	sub.Value = req.Value
	sub.Enabled = req.Enabled

	// Sync to Redis (update user data)
	acc, _ := accountRepo.FindByID(claims.UserID)
	if acc != nil {
		go redisSync.SyncSubscriptionCreate(sub, acc)
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "訂閱已更新"})
}

// DeleteSubscription deletes a subscription
func DeleteSubscription(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	id, err := strconv.Atoi(ps.ByName("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱 ID"})
		return
	}

	// Check ownership
	sub, err := subscriptionRepo.FindByID(id)
	if err != nil {
		if err == account.ErrSubscriptionNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱失敗"})
		return
	}

	if sub.UserID != claims.UserID && claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "禁止存取"})
		return
	}

	// Get account for Redis sync before deleting
	acc, _ := accountRepo.FindByID(claims.UserID)

	if err := subscriptionRepo.Delete(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "刪除訂閱失敗"})
		return
	}

	// Sync to Redis (remove from Redis)
	if acc != nil {
		go redisSync.SyncSubscriptionDelete(sub, acc)
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "訂閱已刪除"})
}
