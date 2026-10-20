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
	triggerIndex map[string]lanes.TriggerTarget
	checkpoints  map[string]*checkpoints.Checkpoint
	gateLanes    map[string]*lanes.Lane
	gateBarriers map[string]map[string]*devices.Barrier
}

func New(
	triggerIndex map[string]lanes.TriggerTarget,
	siteCheckpoints map[string]*checkpoints.Checkpoint,
	gateLanes map[string]*lanes.Lane,
	gateBarriers map[string]map[string]*devices.Barrier,
) *API {
	return &API{
		triggerIndex: triggerIndex,
		checkpoints:  siteCheckpoints,
		gateLanes:    gateLanes,
		gateBarriers: gateBarriers,
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/checkpoints", a.checkpointList)
	mux.HandleFunc("/checkpoints/", a.checkpointHandler)

	mux.HandleFunc(
		"POST /api/v1/inEvents/keyboardMessage",
		a.codeTrigger,
	)

	mux.HandleFunc(
		"GET /api/v1/inEvents/plate",
		a.plateTrigger,
	)

	mux.HandleFunc(
		"GET /api/v1/autogate/context",
		a.withGateName(a.laneContext),
	)

	mux.HandleFunc(
		"GET /api/v1/autogate/photo/last",
		a.withGateName(a.lanePhotoLast),
	)

	mux.HandleFunc(
		"POST /api/v1/autogate/command",
		a.withGateName(a.laneCommand),
	)

	mux.HandleFunc(
		"GET /api/v1/UDTGate/{barrier}/state",
		a.withGateName(a.barrierState),
	)

	mux.HandleFunc(
		"GET /api/v1/UDTGate/{$}",
		a.withGateName(a.barrierList),
	)

	mux.HandleFunc(
		"GET /api/v1/UDTGate/count",
		a.withGateName(a.barrierCount),
	)

	mux.HandleFunc(
		"GET /api/v1/UDTGate/service/status",
		a.withGateName(a.barrierServiceStatus),
	)

	mux.HandleFunc(
		"POST /api/v1/UDTGate/service/{command}",
		a.withGateName(a.barrierServiceCommand),
	)

	mux.HandleFunc(
		"GET /api/v1/UDTGate/{barrier}",
		a.withGateName(a.barrierInfo),
	)

	mux.HandleFunc(
		"GET /api/v1/UDTGate/{barrier}/alarm",
		a.withGateName(a.barrierAlarm),
	)

	mux.HandleFunc(
		"POST /api/v1/UDTGate/{barrier}/{command}",
		a.withGateName(a.barrierCommand),
	)

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
