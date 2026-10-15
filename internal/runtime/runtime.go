package runtime

import (
	"AutoGo/internal/access"
	"AutoGo/internal/axxon"
	"AutoGo/internal/checkpoints"
	"AutoGo/internal/config"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plcclient"
)

type Runtime struct {
	AxxonServices map[string]*axxon.Client

	Controllers  map[string]*plcclient.Client
	Barriers     map[string]*devices.Barrier
	Lanes        map[string]*lanes.Lane
	BarrierLane  map[string]*lanes.Lane
	TriggerIndex map[string]lanes.TriggerTarget
	Checkpoints  map[string]*checkpoints.Checkpoint
	GateLanes    map[string]*lanes.Lane
}

func Build(cfg config.Config) (*Runtime, error) {
	axxonServices, err := buildAxxonClients(cfg.Services)
	if err != nil {
		return nil, err
	}

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
		axxonServices,
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

	gateLanes := make(map[string]*lanes.Lane, len(siteLanes))

	for _, checkpoint := range cfg.Checkpoints {
		for _, laneConfig := range checkpoint.Lanes {
			gateLanes[laneConfig.GateName] = siteLanes[laneConfig.ID]
		}
	}

	return &Runtime{
		AxxonServices: axxonServices,
		Controllers:   controllers,
		Barriers:      barriers,
		Lanes:         siteLanes,
		BarrierLane:   barrierLane,
		TriggerIndex:  triggerIndex,
		Checkpoints:   siteCheckpoints,
		GateLanes:     gateLanes,
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
