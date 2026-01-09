package api

import (
	"encoding/json"
	"net/http"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/mail"
	"github.com/julienschmidt/httprouter"
)

var pttAccountRepo = &account.PTTAccountPostgres{}

// PTTAccountRequest represents a request to bind PTT account
type PTTAccountRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BindPTTAccount binds a PTT account to the user
func BindPTTAccount(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	// Check if user has VIP or above role
	if claims.Role != "vip" && claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Success: false, Message: "此功能僅限 VIP 以上用戶使用"})
		return
	}

	var req PTTAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的請求內容"})
		return
	}

	// Validate
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "PTT 帳號和密碼為必填"})
		return
	}

	// Test PTT login
	client := mail.NewPTTClient(req.Username, req.Password)
	if err := client.SendMail("", "", ""); err != nil {
		// We expect this to fail since we're not actually sending mail
		// But if it's a login error, we should report it
		if err == mail.ErrLoginFailed {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "PTT 帳號或密碼錯誤"})
			return
		}
	}

	// Check if already bound
	existing, _ := pttAccountRepo.FindByUserID(claims.UserID)
	if existing != nil {
		// Update existing
		_, err := pttAccountRepo.Update(claims.UserID, req.Username, req.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "更新 PTT 帳號失敗"})
			return
		}
		writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "PTT 帳號已更新"})
		return
	}

	// Create new binding
	_, err := pttAccountRepo.Create(claims.UserID, req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "綁定 PTT 帳號失敗"})
		return
	}

	writeJSON(w, http.StatusCreated, SuccessResponse{Success: true, Message: "PTT 帳號已綁定"})
}

// UnbindPTTAccount unbinds the PTT account from the user
func UnbindPTTAccount(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Success: false, Message: "未授權"})
		return
	}

	err := pttAccountRepo.Delete(claims.UserID)
	if err != nil {
		if err == account.ErrPTTAccountNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Success: false, Message: "尚未綁定 PTT 帳號"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "解除綁定失敗"})
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{Success: true, Message: "已解除 PTT 帳號綁定"})
}

// SendPTTMail sends a PTT mail (called from Telegram callback)
func SendPTTMail(userID int, recipient, subject, content string) error {
	// Get PTT credentials
	username, password, err := pttAccountRepo.GetCredentials(userID)
	if err != nil {
		return err
	}

	// Send mail
	return mail.SendMail(username, password, recipient, subject, content)
}
