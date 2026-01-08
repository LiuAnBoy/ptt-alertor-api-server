package api

import (
	"net/http"
	"strconv"

	"github.com/Ptt-Alertor/ptt-alertor/models/account"
	"github.com/julienschmidt/httprouter"
)

var statsRepo = &account.SubscriptionStatsPostgres{}

// ListSubscriptionStats returns subscription statistics
func ListSubscriptionStats(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	subType := r.URL.Query().Get("type")
	if subType == "" {
		subType = "keyword" // default to keyword
	}

	// Validate sub_type
	validSubTypes := map[string]bool{"keyword": true, "author": true, "pushsum": true}
	if !validSubTypes[subType] {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Success: false, Message: "無效的類型，必須是 keyword、author 或 pushsum"})
		return
	}

	// Parse limit
	limit := 100 // default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Parse board filter
	board := r.URL.Query().Get("board")

	var stats []*account.SubscriptionStat
	var err error

	if board != "" {
		stats, err = statsRepo.ListByBoard(board, limit)
	} else {
		stats, err = statsRepo.ListByType(subType, limit)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Success: false, Message: "取得統計資料失敗"})
		return
	}

	if stats == nil {
		stats = []*account.SubscriptionStat{}
	}

	writeJSON(w, http.StatusOK, stats)
}
