package runtime

import (
	"AutoGo/internal/access"
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
	deviceConfigs []config.DeviceConfig,
	barriers map[string]*devices.Barrier,
	decider access.Decider,
) (
	map[string]*lanes.Lane,
	map[string]*lanes.Lane,
	map[string]*lanes.Lane,
	error,
) {
	deviceByID := make(
		map[string]config.DeviceConfig,
		len(deviceConfigs),
	)

	for _, device := range deviceConfigs {
		deviceByID[device.ID] = device
	}

	result := make(map[string]*lanes.Lane)
	barrierLane := make(map[string]*lanes.Lane)
	triggerIndex := make(map[string]*lanes.Lane)

	for _, checkpoint := range checkpoints {
		for _, laneConfig := range checkpoint.Lanes {
			switch laneConfig.Scenario.Type {
			case config.ScenarioTypeSingleBarrier:
				barrierID := laneConfig.Scenario.Settings.Barrier

				barrier, exists := barriers[barrierID]
				if !exists {
					return nil, nil, nil, fmt.Errorf(
						"для линии %q не найден шлагбаум %q",
						laneConfig.ID,
						barrierID,
					)
				}

				scenario, err := scenarios.NewSingleBarrier(
					barrier,
					laneConfig.Scenario.Settings.ReleaseMode,
					laneConfig.Scenario.Settings.Direction,
				)
				if err != nil {
					return nil, nil, nil, fmt.Errorf(
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
					decider,
				)
				if err != nil {
					return nil, nil, nil, err
				}

				result[lane.ID] = lane

				for _, deviceID := range laneConfig.Devices {
					barrierLane[deviceID] = lane
				}

				for _, deviceID := range laneConfig.Devices {
					device, ok := deviceByID[deviceID]
					if !ok || device.ExternalID == "" {
						continue
					}

					key := device.Type + ":" + device.ExternalID

					if existing, exists := triggerIndex[key]; exists {
						return nil, nil, nil, fmt.Errorf(
							"источник %q уже привязан к линии %q, повторно у линии %q",
							key,
							existing.ID,
							lane.ID,
						)
					}

					triggerIndex[key] = lane
				}

			default:
				return nil, nil, nil, fmt.Errorf(
					"линия %q использует неизвестный сценарий %q",
					laneConfig.ID,
					laneConfig.Scenario.Type,
				)
			}
		}
	}

	if len(result) == 0 {
		return nil, nil, nil, errors.New(
			"в конфигурации нет включённых линий",
		)
	}

	return result, barrierLane, triggerIndex, nil
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
