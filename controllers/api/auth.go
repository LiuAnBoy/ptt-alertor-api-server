package api

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/Ptt-Alertor/ptt-alertor/models/binding"
	"github.com/julienschmidt/httprouter"
)

var accountRepo = &account.Postgres{}
var bindingRepo = &binding.Postgres{}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents an auth response with token
type AuthResponse struct {
	Token   string           `json:"token"`
	Account *account.Account `json:"account"`
}

// MessageResponse represents a simple message response
type MessageResponse struct {
	Message string `json:"message"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// isValidEmail validates email format
func isValidEmail(email string) bool {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)
	return matched
}

// Register handles user registration
func Register(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的請求內容"})
		return
	}

	// Validate email
	if !isValidEmail(req.Email) {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的電子郵件格式"})
		return
	}

	// Validate password
	if len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "密碼必須至少 6 個字元"})
		return
	}

	// Hash password
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "密碼加密失敗"})
		return
	}

	// Create account
	acc, err := accountRepo.Create(req.Email, hash, "user")
	if err != nil {
		if err == account.ErrEmailExists {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "電子郵件已存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "建立帳號失敗"})
		return
	}

	// Generate token
	token, err := auth.GenerateToken(acc.ID, acc.Email, acc.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "產生令牌失敗"})
		return
	}

	writeJSON(w, http.StatusCreated, AuthResponse{
		Token:   token,
		Account: acc,
	})
}

// Login handles user login
func Login(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的請求內容"})
		return
	}

	// Find account
	acc, err := accountRepo.FindByEmail(req.Email)
	if err != nil {
		if err == account.ErrAccountNotFound {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "電子郵件或密碼錯誤"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "查詢帳號失敗"})
		return
	}

	// Check if account is enabled
	if !acc.Enabled {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "帳號已停用"})
		return
	}

	// Check password
	if !auth.CheckPassword(req.Password, acc.Password) {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "電子郵件或密碼錯誤"})
		return
	}

	// Generate token
	token, err := auth.GenerateToken(acc.ID, acc.Email, acc.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "產生令牌失敗"})
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		Token:   token,
		Account: acc,
	})
}

// MeResponse represents the /me endpoint response
type MeResponse struct {
	ID        int             `json:"id"`
	Email     string          `json:"email"`
	Role      string          `json:"role"`
	Bindings  map[string]bool `json:"bindings"`
	Enabled   bool            `json:"enabled"`
	CreatedAt string          `json:"created_at"`
}

// Me returns the current user info
func Me(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
		return
	}

	acc, err := accountRepo.FindByID(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "找不到帳號"})
		return
	}

	// Get all bindings for this account
	bindings, _ := bindingRepo.FindAllByUser(acc.ID)
	bindingStatus := map[string]bool{
		binding.ServiceTelegram: false,
		binding.ServiceLine:     false,
		binding.ServiceDiscord:  false,
	}
	for _, b := range bindings {
		if b.ServiceID != "" && b.Enabled {
			bindingStatus[b.Service] = true
		}
	}

	response := MeResponse{
		ID:        acc.ID,
		Email:     acc.Email,
		Role:      acc.Role,
		Bindings:  bindingStatus,
		Enabled:   acc.Enabled,
		CreatedAt: acc.CreatedAt.Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, response)
}

// GenerateBindCodeRequest represents a request to generate bind code
type GenerateBindCodeRequest struct {
	Service string `json:"service"`
}

// GenerateBindCode generates a new bind code for a notification service
func GenerateBindCode(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
		return
	}

	var req GenerateBindCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的請求內容"})
		return
	}

	// Default to telegram for backward compatibility
	service := req.Service
	if service == "" {
		service = binding.ServiceTelegram
	}

	// Validate service type
	if service != binding.ServiceTelegram && service != binding.ServiceLine && service != binding.ServiceDiscord {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的服務類型"})
		return
	}

	// Check if already bound
	existingBinding, err := bindingRepo.FindByUserAndService(claims.UserID, service)
	if err == nil && existingBinding.ServiceID != "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "此服務已綁定"})
		return
	}

	// Generate bind code
	code, err := binding.GenerateBindCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "產生綁定碼失敗"})
		return
	}

	// Save bind code (expires in 10 minutes)
	expiresAt := time.Now().Add(10 * time.Minute)
	if err := bindingRepo.UpdateBindCode(claims.UserID, service, code, expiresAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "儲存綁定碼失敗"})
		return
	}

	response := map[string]interface{}{
		"bind_code":  code,
		"expires_at": expiresAt,
		"service":    service,
	}

	// Generate deep link for supported services
	switch service {
	case binding.ServiceTelegram:
		botUsername := os.Getenv("TELEGRAM_BOT_USERNAME")
		if botUsername != "" {
			response["bind_url"] = "https://t.me/" + botUsername + "?start=BIND_" + code
			response["message"] = "點擊連結完成綁定"
		} else {
			response["message"] = "請在 Telegram 機器人輸入 /bind " + code
		}
	default:
		response["message"] = "請在 " + service + " 機器人輸入 /bind " + code
	}

	writeJSON(w, http.StatusOK, response)
}

// BindingStatusRequest represents a request to get binding status
type BindingStatusRequest struct {
	Service string `json:"service"`
}

// BindingStatus returns notification service binding status
func BindingStatus(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
		return
	}

	service := ps.ByName("service")
	if service == "" {
		service = binding.ServiceTelegram
	}

	b, err := bindingRepo.FindByUserAndService(claims.UserID, service)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service":    service,
			"bound":      false,
			"service_id": nil,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service":    service,
		"bound":      b.ServiceID != "" && b.Enabled,
		"service_id": b.ServiceID,
		"enabled":    b.Enabled,
	})
}

// UnbindRequest represents a request to unbind a service
type UnbindRequest struct {
	Service string `json:"service"`
}

// UnbindService unbinds a notification service from account
func UnbindService(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
		return
	}

	service := ps.ByName("service")
	if service == "" {
		service = binding.ServiceTelegram
	}

	if err := bindingRepo.Delete(claims.UserID, service); err != nil {
		if err == binding.ErrBindingNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "找不到綁定"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "解除綁定失敗"})
		return
	}

	writeJSON(w, http.StatusOK, MessageResponse{Message: "已成功解除綁定"})
}

// GetAllBindings returns all bindings for the current user
func GetAllBindings(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
		return
	}

	bindings, err := bindingRepo.FindAllByUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "查詢綁定失敗"})
		return
	}

	writeJSON(w, http.StatusOK, bindings)
}

// SetBindingEnabledRequest represents a request to enable/disable binding
type SetBindingEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// SetBindingEnabled enables or disables a notification binding
func SetBindingEnabled(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
		return
	}

	service := ps.ByName("service")
	if service == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "缺少服務類型"})
		return
	}

	var req SetBindingEnabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "無效的請求內容"})
		return
	}

	if err := bindingRepo.SetEnabled(claims.UserID, service, req.Enabled); err != nil {
		if err == binding.ErrBindingNotFound {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "找不到綁定"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "更新綁定失敗"})
		return
	}

	status := "停用"
	if req.Enabled {
		status = "啟用"
	}
	writeJSON(w, http.StatusOK, MessageResponse{Message: "已" + status + "綁定"})
}

