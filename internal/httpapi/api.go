package httpapi

import (
	"log"
	"net/http"
	"time"

	"AutoGo/internal/checkpoints"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
)

type API struct {
	barriers     map[string]*devices.Barrier
	lanes        map[string]*lanes.Lane
	barrierLane  map[string]*lanes.Lane
	triggerIndex map[string]lanes.TriggerTarget
	checkpoints  map[string]*checkpoints.Checkpoint
}

func New(
	barriers map[string]*devices.Barrier,
	siteLanes map[string]*lanes.Lane,
	barrierLane map[string]*lanes.Lane,
	triggerIndex map[string]lanes.TriggerTarget,
	siteCheckpoints map[string]*checkpoints.Checkpoint,
) *API {
	return &API{
		barriers:     barriers,
		lanes:        siteLanes,
		barrierLane:  barrierLane,
		triggerIndex: triggerIndex,
		checkpoints:  siteCheckpoints,
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/checkpoints", a.checkpointList)
	mux.HandleFunc("/checkpoints/", a.checkpointHandler)
	mux.HandleFunc("/barriers/", a.barrierHandler)
	mux.HandleFunc("/lanes/", a.laneHandler)
	mux.HandleFunc("/plate", a.plateTrigger)
	mux.HandleFunc("/code", a.codeTrigger)

	return logRequests(mux)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		target := r.URL.Path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}

		log.Printf(
			"HTTP %s %s -> %d (%s)",
			r.Method,
			target,
			rec.status,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
