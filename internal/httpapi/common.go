package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
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
