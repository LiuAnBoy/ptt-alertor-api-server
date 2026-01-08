package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	"github.com/Ptt-Alertor/ptt-alertor/models/binding"
	"github.com/julienschmidt/httprouter"
)

// TelegramWebAppRequest represents a request from Telegram Web App
type TelegramWebAppRequest struct {
	Token string `json:"token"`
}

// TelegramWebAppConfirm confirms telegram binding from Web App
// The token was generated when user typed /bind in Telegram and stored in Redis
func TelegramWebAppConfirm(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// Get user from JWT
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	var req TelegramWebAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "缺少 token"})
		return
	}

	// Get chat_id from Redis using token
	chatID, err := telegram.GetWebAppBindChatID(req.Token)
	if err != nil {
		if err == redis.ErrNil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效或已過期的 token"})
			return
		}
		log.WithError(err).Error("Failed to get chat_id from Redis")
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "查詢失敗"})
		return
	}

	chatIDStr := strconv.FormatInt(chatID, 10)

	// Check if user already has telegram binding
	existingBinding, err := bindingRepo.FindByUserAndService(claims.UserID, binding.ServiceTelegram)
	if err == nil && existingBinding.ServiceID != "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "此帳號已綁定 Telegram"})
		return
	}

	// Check if this chat_id is already bound to another user
	existingByServiceID, err := bindingRepo.FindByServiceID(binding.ServiceTelegram, chatIDStr)
	if err == nil && existingByServiceID != nil && existingByServiceID.UserID != claims.UserID {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "此 Telegram 已綁定其他帳號"})
		return
	}

	// Create binding for current user
	err = bindingRepo.ConfirmBinding(claims.UserID, binding.ServiceTelegram, chatIDStr)
	if err != nil {
		log.WithError(err).Error("Failed to confirm binding")
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "綁定失敗"})
		return
	}

	// Sync existing subscriptions to Redis after binding
	go redisSync.SyncAllSubscriptions(claims.UserID)

	// Send success message to Telegram
	telegram.SendTextMessage(chatID, "綁定成功！您現在可以在網頁上管理訂閱，通知將發送到此 Telegram。")

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "綁定成功"})
}
