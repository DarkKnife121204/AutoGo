package httpapi

import (
	"net/http"
	"strings"

	"AutoGo/internal/lanes"
)

func (a *API) withGateName(
	handler func(http.ResponseWriter, *http.Request, *lanes.Lane),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gateName := strings.TrimSpace(
			r.Header.Get("X-Gate-Name"),
		)

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

func (a *API) compatPhotoLast(
	w http.ResponseWriter,
	_ *http.Request,
	lane *lanes.Lane,
) {
	a.writeLanePhotoLast(w, lane)
}
