package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/julienschmidt/httprouter"
)

var subscriptionRepo = &account.SubscriptionPostgres{}

// MailTemplateRequest represents mail template in request
type MailTemplateRequest struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
}

// CreateSubscriptionRequest represents a subscription creation request
type CreateSubscriptionRequest struct {
	Board   string               `json:"board"`
	SubType string               `json:"sub_type"`
	Value   string               `json:"value"`
	Mail    *MailTemplateRequest `json:"mail,omitempty"`
}

// UpdateSubscriptionRequest represents a subscription update request
type UpdateSubscriptionRequest struct {
	Board   string               `json:"board"`
	SubType string               `json:"sub_type"`
	Value   string               `json:"value"`
	Enabled bool                 `json:"enabled"`
	Mail    *MailTemplateRequest `json:"mail,omitempty"`
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

	// Basic validation
	if req.Board == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板為必填"})
		return
	}

	validSubTypes := map[string]bool{"keyword": true, "author": true, "pushsum": true}
	if !validSubTypes[req.SubType] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱類型，必須是 keyword、author 或 pushsum"})
		return
	}

	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "訂閱值為必填"})
		return
	}

	// Create subscription (includes limit check, board validation, Redis sync, stats)
	_, err := subscriptionRepo.Create(claims.UserID, req.Board, req.SubType, req.Value)
	if err != nil {
		switch err {
		case account.ErrSubscriptionExists:
			writeJSON(w, http.StatusConflict, ErrorResponse{Success: false, Message: "訂閱已存在"})
		case account.ErrSubscriptionLimitReached:
			writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "已達訂閱上限"})
		case account.ErrBoardNotFound:
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板不存在"})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "建立訂閱失敗"})
		}
		return
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

	var req UpdateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Basic validation
	if req.Board == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板為必填"})
		return
	}

	validSubTypes := map[string]bool{"keyword": true, "author": true, "pushsum": true}
	if !validSubTypes[req.SubType] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的訂閱類型，必須是 keyword、author 或 pushsum"})
		return
	}

	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "訂閱值為必填"})
		return
	}

	// Prepare mail template pointers
	var mailSubject, mailContent *string
	if req.Mail != nil {
		if req.Mail.Subject != "" {
			mailSubject = &req.Mail.Subject
		}
		if req.Mail.Content != "" {
			mailContent = &req.Mail.Content
		}
	}

	// Update subscription (includes ownership check, board validation, Redis sync, stats)
	// Admin can update any subscription
	userID := claims.UserID
	if claims.Role == "admin" {
		// For admin, get the actual owner's userID from the subscription
		sub, err := subscriptionRepo.FindByID(id)
		if err != nil {
			if err == account.ErrSubscriptionNotFound {
				writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱失敗"})
			return
		}
		userID = sub.UserID
	}

	err = subscriptionRepo.Update(id, userID, req.Board, req.SubType, req.Value, req.Enabled, mailSubject, mailContent)
	if err != nil {
		switch err {
		case account.ErrSubscriptionNotFound:
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
		case account.ErrBoardNotFound:
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "看板不存在"})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "更新訂閱失敗"})
		}
		return
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

	// For admin, get the actual owner's userID from the subscription
	userID := claims.UserID
	if claims.Role == "admin" {
		sub, err := subscriptionRepo.FindByID(id)
		if err != nil {
			if err == account.ErrSubscriptionNotFound {
				writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得訂閱失敗"})
			return
		}
		userID = sub.UserID
	}

	// Delete subscription (includes ownership check, Redis sync, stats)
	err = subscriptionRepo.Delete(id, userID)
	if err != nil {
		if err == account.ErrSubscriptionNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "找不到訂閱"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "刪除訂閱失敗"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "訂閱已刪除"})
}
