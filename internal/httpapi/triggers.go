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

	a.dispatchPlateTrigger(
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
	var request codeTriggerRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Ошибка данных",
		})
		return
	}

	sender := strings.TrimSpace(request.Sender)
	content := strings.TrimSpace(request.Content)
	title := strings.TrimSpace(request.Title)

	if sender == "" || content == "" || title == "" ||
		len([]rune(request.Title)) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Ошибка данных",
		})
		return
	}

	a.dispatchKeyCodeTrigger(
		w,
		"keypad:"+sender,
		scenarios.TriggerSourceCode,
		content,
	)
}

func (a *API) dispatchPlateTrigger(
	w http.ResponseWriter,
	indexKey string,
	source scenarios.TriggerSource,
	plate string,
) {
	target, exists := a.triggerIndex[indexKey]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Камера не найдена",
		})
		return
	}

	if err := target.Lane.TriggerPlate(
		source,
		plate,
		target.Direction,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Внутренняя ошибка сервера",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}

func (a *API) dispatchKeyCodeTrigger(
	w http.ResponseWriter,
	indexKey string,
	source scenarios.TriggerSource,
	keyCode string,
) {
	target, exists := a.triggerIndex[indexKey]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "Клавиатура не найдена",
		})
		return
	}

	if err := target.Lane.TriggerKeyCode(
		source,
		keyCode,
		target.Direction,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Внутренняя ошибка сервера",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}
