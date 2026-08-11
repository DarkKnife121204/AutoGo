package httpapi

import (
	"net/http"
	"sort"
	"strings"

	"AutoGo/internal/checkpoints"
)

func (a *API) checkpointList(
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

	checkpointIDs := make(
		[]string,
		0,
		len(a.checkpoints),
	)

	for checkpointID := range a.checkpoints {
		checkpointIDs = append(
			checkpointIDs,
			checkpointID,
		)
	}

	sort.Strings(checkpointIDs)

	response := checkpointListResponse{
		Status: "ok",
		Checkpoints: make(
			[]checkpointResponse,
			0,
			len(checkpointIDs),
		),
	}

	for _, checkpointID := range checkpointIDs {
		response.Checkpoints = append(
			response.Checkpoints,
			makeCheckpointResponse(
				a.checkpoints[checkpointID],
			),
		)
	}

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

func (a *API) checkpointHandler(
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

	checkpointID := strings.Trim(
		strings.TrimPrefix(
			r.URL.Path,
			"/checkpoints/",
		),
		"/",
	)

	if checkpointID == "" ||
		strings.Contains(checkpointID, "/") {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "route not found",
			},
		)

		return
	}

	checkpoint, exists := a.checkpoints[checkpointID]
	if !exists {
		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"status": "error",
				"error":  "checkpoint not found",
			},
		)

		return
	}

	writeJSON(
		w,
		http.StatusOK,
		struct {
			Status     string             `json:"status"`
			Checkpoint checkpointResponse `json:"checkpoint"`
		}{
			Status: "ok",
			Checkpoint: makeCheckpointResponse(
				checkpoint,
			),
		},
	)
}

func makeCheckpointResponse(
	checkpoint *checkpoints.Checkpoint,
) checkpointResponse {
	laneIDs := make(
		[]string,
		0,
		len(checkpoint.Lanes),
	)

	for laneID := range checkpoint.Lanes {
		laneIDs = append(
			laneIDs,
			laneID,
		)
	}

	sort.Strings(laneIDs)

	response := checkpointResponse{
		ID:   checkpoint.ID,
		Name: checkpoint.Name,
		Lanes: make(
			[]laneSummaryResponse,
			0,
			len(laneIDs),
		),
	}

	for _, laneID := range laneIDs {
		lane := checkpoint.Lanes[laneID]

		response.Lanes = append(
			response.Lanes,
			laneSummaryResponse{
				ID:       lane.ID,
				Name:     lane.Name,
				Scenario: lane.ScenarioType(),
			},
		)
	}

	return response
}
