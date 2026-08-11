package runtime

import (
	"errors"
	"fmt"
	"log"

	"AutoGo/internal/checkpoints"
	"AutoGo/internal/config"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plcclient"
	"AutoGo/internal/scenarios"
)

func buildPLCClients(
	controllers []config.ControllerConfig,
) (map[string]*plcclient.Client, error) {
	clients := make(
		map[string]*plcclient.Client,
		len(controllers),
	)

	for _, controller := range controllers {
		if !controller.Enabled {
			continue
		}

		client, err := plcclient.New(
			plcclient.Config{
				Address: controller.Address,
				UnitID:  controller.UnitID,
				Timeout: controller.Timeout,
			},
		)
		if err != nil {
			closePLCClients(clients)

			return nil, fmt.Errorf(
				"создание PLC-клиента %q: %w",
				controller.ID,
				err,
			)
		}

		clients[controller.ID] = client
	}

	if len(clients) == 0 {
		return nil, errors.New(
			"в конфигурации нет включённых PLC-контроллеров",
		)
	}

	return clients, nil
}

func closePLCClients(
	clients map[string]*plcclient.Client,
) {
	for controllerID, client := range clients {
		if err := client.Close(); err != nil {
			log.Printf(
				"ошибка закрытия PLC-клиента %s: %v",
				controllerID,
				err,
			)
		}
	}
}

func buildBarriers(
	deviceConfigs []config.DeviceConfig,
	controllers map[string]*plcclient.Client,
) (map[string]*devices.Barrier, error) {
	barriers := make(map[string]*devices.Barrier)

	for _, deviceConfig := range deviceConfigs {
		if !deviceConfig.Enabled {
			continue
		}

		if deviceConfig.Type != config.DeviceTypeBarrier {
			continue
		}

		controller, exists := controllers[deviceConfig.Controller]
		if !exists {
			return nil, fmt.Errorf(
				"для шлагбаума %q не найден контроллер %q",
				deviceConfig.ID,
				deviceConfig.Controller,
			)
		}

		barrier, err := devices.NewBarrier(
			deviceConfig.ID,
			deviceConfig.Name,
			deviceConfig.Controller,
			controller,
		)
		if err != nil {
			return nil, err
		}

		barriers[deviceConfig.ID] = barrier
	}

	if len(barriers) == 0 {
		return nil, errors.New(
			"в конфигурации нет включённых шлагбаумов",
		)
	}

	return barriers, nil
}

func buildLanes(
	checkpoints []config.CheckpointConfig,
	barriers map[string]*devices.Barrier,
) (
	map[string]*lanes.Lane,
	map[string]*lanes.Lane,
	error,
) {
	result := make(map[string]*lanes.Lane)
	barrierLane := make(map[string]*lanes.Lane)

	for _, checkpoint := range checkpoints {
		for _, laneConfig := range checkpoint.Lanes {
			if !laneConfig.Enabled {
				continue
			}

			switch laneConfig.Scenario.Type {
			case config.ScenarioTypeSingleBarrier:
				barrierID := laneConfig.Scenario.Settings.Barrier

				barrier, exists := barriers[barrierID]
				if !exists {
					return nil, nil, fmt.Errorf(
						"для линии %q не найден шлагбаум %q",
						laneConfig.ID,
						barrierID,
					)
				}

				triggers := laneConfig.Scenario.Settings.Triggers
				if len(triggers) == 0 {
					triggers = []string{config.TriggerOperator}
				}

				scenario, err := scenarios.NewSingleBarrier(
					barrier,
					laneConfig.Scenario.Settings.ReleaseMode,
					laneConfig.Scenario.Settings.Direction,
					triggers,
				)
				if err != nil {
					return nil, nil, fmt.Errorf(
						"создание сценария линии %q: %w",
						laneConfig.ID,
						err,
					)
				}

				lane, err := lanes.New(
					laneConfig.ID,
					laneConfig.Name,
					checkpoint.ID,
					scenario,
					lanes.ParseMode(laneConfig.Mode),
				)
				if err != nil {
					return nil, nil, err
				}

				result[lane.ID] = lane

				for _, deviceID := range laneConfig.Devices {
					barrierLane[deviceID] = lane
				}

			default:
				return nil, nil, fmt.Errorf(
					"линия %q использует неизвестный сценарий %q",
					laneConfig.ID,
					laneConfig.Scenario.Type,
				)
			}
		}
	}

	if len(result) == 0 {
		return nil, nil, errors.New(
			"в конфигурации нет включённых линий",
		)
	}

	return result, barrierLane, nil
}

func buildCheckpoints(
	configs []config.CheckpointConfig,
	siteLanes map[string]*lanes.Lane,
) (map[string]*checkpoints.Checkpoint, error) {
	result := make(
		map[string]*checkpoints.Checkpoint,
		len(configs),
	)

	for _, checkpointConfig := range configs {
		checkpointLanes := make(
			map[string]*lanes.Lane,
		)

		for _, laneConfig := range checkpointConfig.Lanes {
			if !laneConfig.Enabled {
				continue
			}

			lane, exists := siteLanes[laneConfig.ID]
			if !exists {
				return nil, fmt.Errorf(
					"для КПП %q не найдена линия %q",
					checkpointConfig.ID,
					laneConfig.ID,
				)
			}

			checkpointLanes[lane.ID] = lane
		}

		checkpoint, err := checkpoints.New(
			checkpointConfig.ID,
			checkpointConfig.Name,
			checkpointLanes,
		)
		if err != nil {
			return nil, err
		}

		result[checkpoint.ID] = checkpoint
	}

	if len(result) == 0 {
		return nil, errors.New(
			"в конфигурации нет КПП с включёнными линиями",
		)
	}

	return result, nil
}
