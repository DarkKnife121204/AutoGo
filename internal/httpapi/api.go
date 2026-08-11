package httpapi

import (
	"net/http"

	"AutoGo/internal/checkpoints"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
)

type API struct {
	barriers    map[string]*devices.Barrier
	lanes       map[string]*lanes.Lane
	barrierLane map[string]*lanes.Lane
	checkpoints map[string]*checkpoints.Checkpoint
}

func New(
	barriers map[string]*devices.Barrier,
	siteLanes map[string]*lanes.Lane,
	barrierLane map[string]*lanes.Lane,
	siteCheckpoints map[string]*checkpoints.Checkpoint,
) *API {
	return &API{
		barriers:    barriers,
		lanes:       siteLanes,
		barrierLane: barrierLane,
		checkpoints: siteCheckpoints,
	}
}

// Routes возвращает готовый HTTP-обработчик со всеми маршрутами AutoGo.
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/checkpoints", a.checkpointList)
	mux.HandleFunc("/checkpoints/", a.checkpointHandler)
	mux.HandleFunc("/barriers/", a.barrierHandler)
	mux.HandleFunc("/lanes/", a.laneHandler)

	return mux
}
