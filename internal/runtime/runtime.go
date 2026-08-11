package runtime

import (
	"AutoGo/internal/access"
	"AutoGo/internal/checkpoints"
	"AutoGo/internal/config"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plcclient"
)

type Runtime struct {
	Controllers  map[string]*plcclient.Client
	Barriers     map[string]*devices.Barrier
	Lanes        map[string]*lanes.Lane
	BarrierLane  map[string]*lanes.Lane
	TriggerIndex map[string]*lanes.Lane
	Checkpoints  map[string]*checkpoints.Checkpoint
}

func Build(cfg config.Config) (*Runtime, error) {
	controllers, err := buildPLCClients(cfg.Controllers)
	if err != nil {
		return nil, err
	}

	barriers, err := buildBarriers(cfg.Devices, controllers)
	if err != nil {
		closePLCClients(controllers)

		return nil, err
	}

	decider := access.NewStubDecider()

	siteLanes, barrierLane, triggerIndex, err := buildLanes(
		cfg.Checkpoints,
		cfg.Devices,
		barriers,
		decider,
	)
	if err != nil {
		closePLCClients(controllers)

		return nil, err
	}

	siteCheckpoints, err := buildCheckpoints(cfg.Checkpoints, siteLanes)
	if err != nil {
		closePLCClients(controllers)

		return nil, err
	}

	return &Runtime{
		Controllers:  controllers,
		Barriers:     barriers,
		Lanes:        siteLanes,
		BarrierLane:  barrierLane,
		TriggerIndex: triggerIndex,
		Checkpoints:  siteCheckpoints,
	}, nil
}

func (r *Runtime) StartPollers() {
	for _, lane := range r.Lanes {
		lane.StartPoller()
	}
}

func (r *Runtime) StopPollers() {
	for _, lane := range r.Lanes {
		lane.StopPoller()
	}
}

func (r *Runtime) Close() {
	closePLCClients(r.Controllers)
}
