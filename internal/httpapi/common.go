package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"AutoGo/internal/lanes"
)

func (a *API) health(
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

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"status":  "ok",
			"service": "autogo",
		},
	)
}

func writeJSON(
	w http.ResponseWriter,
	statusCode int,
	data any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf(
			"ошибка записи JSON-ответа: %v",
			err,
		)
	}
}

func (a *API) withGateName(
	handler func(http.ResponseWriter, *http.Request, *lanes.Lane),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gateName := strings.TrimSpace(r.Header.Get("X-Gate-Name"))

		if gateName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"errorType": "MissingGateName",
				"message":   "Не указан X-Gate-Name",
			})
			return
		}

		lane, exists := a.gateLanes[gateName]
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"errorType": "GateNotFound",
				"message":   "Линия не найдена: " + gateName,
			})
			return
		}

		handler(w, r, lane)
	}
}
