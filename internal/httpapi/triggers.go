package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"AutoGo/internal/scenarios"
)

func (a *API) plateTrigger(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		writeJSON(
			w,
			http.StatusMethodNotAllowed,
			map[string]string{
				"status": "error",
				"error":  "method not allowed",
			},
		)

		return
	}

	cam := strings.TrimSpace(r.URL.Query().Get("cam"))
	plate := strings.TrimSpace(r.URL.Query().Get("plate"))

	if cam == "" || plate == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "нужны параметры cam и plate",
			},
		)

		return
	}

	a.dispatchTrigger(
		w,
		"camera:"+cam,
		scenarios.TriggerSourceCamera,
		plate,
	)
}

type codeTriggerRequest struct {
	Sender    string `json:"sender"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

func (a *API) codeTrigger(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		writeJSON(
			w,
			http.StatusMethodNotAllowed,
			map[string]string{
				"status": "error",
				"error":  "method not allowed",
			},
		)

		return
	}

	var request codeTriggerRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "invalid JSON body",
			},
		)

		return
	}

	sender := strings.TrimSpace(request.Sender)
	content := strings.TrimSpace(request.Content)

	if sender == "" || content == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"status": "error",
				"error":  "нужны поля sender и content",
			},
		)

		return
	}

	a.dispatchTrigger(
		w,
		"keypad:"+sender,
		scenarios.TriggerSourceCode,
		content,
	)
}

func (a *API) dispatchTrigger(
	w http.ResponseWriter,
	indexKey string,
	source scenarios.TriggerSource,
	value string,
) {
	lane, exists := a.triggerIndex[indexKey]
	if !exists {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "источник не привязан ни к одной линии: " + indexKey,
			},
		)

		return
	}

	if err := lane.Trigger(source, value); err != nil {
		writeJSON(
			w,
			http.StatusConflict,
			map[string]string{
				"status": "error",
				"error":  err.Error(),
			},
		)

		return
	}

	writeJSON(
		w,
		http.StatusAccepted,
		map[string]string{
			"status": "accepted",
			"lane":   lane.ID,
		},
	)
}
