package runtime

import (
	"AutoGo/internal/access"
	"AutoGo/internal/axxon"
	"AutoGo/internal/checkpoints"
	"AutoGo/internal/config"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plcclient"
	"errors"
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
	GateBarriers map[string]map[string]*devices.Barrier
}

func Build(cfg config.Config) (*Runtime, error) {

	var laravelService *config.ServiceConfig

	for i := range cfg.Services {
		if cfg.Services[i].Type != config.ServiceTypeLaravel {
			continue
		}

		if laravelService != nil {
			return nil, errors.New("настроено несколько Laravel-сервисов")
		}

		laravelService = &cfg.Services[i]
	}

	if laravelService == nil {
		return nil, errors.New("Laravel-сервис не настроен")
	}

	gateNames := make(map[string]string)

	for _, checkpoint := range cfg.Checkpoints {
		for _, lane := range checkpoint.Lanes {
			gateNames[lane.ID] = lane.GateName
		}
	}

	decider, err := access.NewLaravelDecider(
		laravelService.Address,
		laravelService.Timeout,
		gateNames,
	)
	if err != nil {
		return nil, err
	}

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

	gateBarriers := make(map[string]map[string]*devices.Barrier)

	for _, checkpoint := range cfg.Checkpoints {
		for _, laneConfig := range checkpoint.Lanes {
			gates := make(map[string]*devices.Barrier)

			switch laneConfig.Scenario.Type {
			case config.ScenarioTypeSingleBarrier:
				gates["Gate1"] = barriers[laneConfig.Scenario.Settings.Barrier]

			case config.ScenarioTypeDoubleBarrier:
				gates["Gate1"] = barriers[laneConfig.Scenario.Settings.EntryBarrier]
				gates["Gate2"] = barriers[laneConfig.Scenario.Settings.ExitBarrier]
			}

			gateBarriers[laneConfig.ID] = gates
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
		GateBarriers:  gateBarriers,
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
