package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BulkRequest is the request body for POST /api/tasks/bulk.
type BulkRequest struct {
	Action     string   `json:"action"`      // "cancel" | "reassign"
	TaskIDs    []string `json:"task_ids"`
	AssignedTo string   `json:"assigned_to"` // required for reassign
}

// BulkResponse summarises the result of a bulk operation.
type BulkResponse struct {
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

// registerBulkRoutes adds the POST /api/tasks/bulk endpoint.
func (h *Handler) registerBulkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/tasks/bulk", h.handleBulk)
}

func (h *Handler) handleBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req BulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.TaskIDs) == 0 {
		writeError(w, http.StatusBadRequest, "task_ids must be non-empty")
		return
	}
	if req.Action != "cancel" && req.Action != "reassign" {
		writeError(w, http.StatusBadRequest, "action must be 'cancel' or 'reassign'")
		return
	}
	if req.Action == "reassign" && strings.TrimSpace(req.AssignedTo) == "" {
		writeError(w, http.StatusBadRequest, "assigned_to is required for reassign")
		return
	}

	resp := BulkResponse{}
	now := time.Now().UTC()

	switch req.Action {
	case "cancel":
		// Cancel non-terminal tasks (pending/claimed/in_progress/blocked/review).
		// This is an admin operation that bypasses the FSM for tasks stuck in intermediate states.
		cancelable := []string{"pending", "claimed", "in_progress", "blocked", "review"}
		placeholders := strings.Repeat(",?", len(req.TaskIDs))[1:]
		inClause := fmt.Sprintf("IN (%s)", placeholders)
		statusIn := fmt.Sprintf("IN ('%s')", strings.Join(cancelable, "','"))

		args := make([]any, 0, len(req.TaskIDs)+2)
		args = append(args, string("cancelled"), now)
		for _, id := range req.TaskIDs {
			args = append(args, id)
		}

		result, err := h.db.Exec(
			fmt.Sprintf(
				`UPDATE tasks SET status = ?, updated_at = ?, version = version+1
				 WHERE id %s AND status %s`,
				inClause, statusIn,
			),
			args...,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		affected, _ := result.RowsAffected()
		resp.Succeeded = int(affected)
		resp.Failed = len(req.TaskIDs) - resp.Succeeded
		if resp.Failed > 0 {
			resp.Errors = append(resp.Errors,
				fmt.Sprintf("%d tasks were not cancelled (already terminal or not found)", resp.Failed))
		}

		// Insert history rows for cancelled tasks.
		if affected > 0 {
			histArgs := make([]any, 0)
			placeholderRows := make([]string, 0)
			for _, id := range req.TaskIDs {
				placeholderRows = append(placeholderRows, "(?, 'cancelled', 'bulk', ?)")
				histArgs = append(histArgs, id, now)
			}
			_, _ = h.db.Exec(
				`INSERT OR IGNORE INTO task_history (task_id, to_status, changed_by, changed_at) VALUES `+
					strings.Join(placeholderRows, ","),
				histArgs...,
			)
		}

	case "reassign":
		for _, id := range req.TaskIDs {
			res, err := h.db.Exec(
				`UPDATE tasks SET assigned_to = ?, updated_at = ?, version = version+1 WHERE id = ?`,
				req.AssignedTo, now, id,
			)
			if err != nil {
				resp.Failed++
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				resp.Failed++
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: not found", id))
			} else {
				resp.Succeeded++
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
