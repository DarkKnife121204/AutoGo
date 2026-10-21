package runtime

import (
	"AutoGo/internal/access"
	"AutoGo/internal/axxon"
	"AutoGo/internal/checkpoints"
	"AutoGo/internal/config"
	"AutoGo/internal/devices"
	"AutoGo/internal/lanes"
	"AutoGo/internal/plcclient"
	"AutoGo/internal/scenarios"
	"AutoGo/internal/video"
	"errors"
	"fmt"
	"log"
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
	axxonServices map[string]*axxon.Client,
	serviceConfigs []config.ServiceConfig,
) (
	map[string]*lanes.Lane,
	map[string]*lanes.Lane,
	map[string]lanes.TriggerTarget,
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
	triggerIndex := make(map[string]lanes.TriggerTarget)

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
					collectSources(laneConfig, deviceByID),
					axxonServices,
					laneConfig.Scenario.Settings.QueueDepth,
					laneConfig.BidirectionalMode,
				)
				if err != nil {
					return nil, nil, nil, err
				}

				result[lane.ID] = lane

				if existing, exists := barrierLane[barrierID]; exists {
					return nil, nil, nil, fmt.Errorf(
						"шлагбаум %q уже привязан к линии %q, повторно у линии %q",
						barrierID,
						existing.ID,
						lane.ID,
					)
				}

				barrierLane[barrierID] = lane

				for _, deviceID := range laneConfig.Devices {
					device, ok := deviceByID[deviceID]
					if !ok || device.ExternalID == "" {
						continue
					}

					key := device.Type + ":" + device.ExternalID
					if existing, exists := triggerIndex[key]; exists {
						return nil, nil, nil, fmt.Errorf(
							"источник %q уже привязан к линии %q, повторно у линии %q",
							key, existing.Lane.ID, lane.ID,
						)
					}
					triggerIndex[key] = lanes.TriggerTarget{
						Lane:      lane,
						Direction: device.Direction,
					}
				}

			case config.ScenarioTypeDoubleBarrier:
				entryID := laneConfig.Scenario.Settings.EntryBarrier
				exitID := laneConfig.Scenario.Settings.ExitBarrier

				entryBarrier, ok1 := barriers[entryID]
				exitBarrier, ok2 := barriers[exitID]

				if !ok1 {
					return nil, nil, nil, fmt.Errorf(
						"для линии %q не найден шлагбаум %q",
						laneConfig.ID, entryID,
					)
				}
				if !ok2 {
					return nil, nil, nil, fmt.Errorf(
						"для линии %q не найден шлагбаум %q",
						laneConfig.ID, exitID,
					)
				}

				scenario, err := scenarios.NewDoubleBarrier(
					entryBarrier,
					exitBarrier,
					laneConfig.Scenario.Settings.ReleaseMode,
				)
				if err != nil {
					return nil, nil, nil, fmt.Errorf(
						"создание сценария линии %q: %w",
						laneConfig.ID, err,
					)
				}

				lane, err := lanes.New(
					laneConfig.ID,
					laneConfig.Name,
					checkpoint.ID,
					scenario,
					lanes.ParseMode(laneConfig.Mode),
					decider,
					collectSources(laneConfig, deviceByID),
					axxonServices,
					laneConfig.Scenario.Settings.QueueDepth,
					laneConfig.BidirectionalMode,
				)
				if err != nil {
					return nil, nil, nil, err
				}

				if err := bindVideoRecorder(lane, laneConfig, axxonServices, serviceConfigs); err != nil {
					return nil, nil, nil, err
				}

				result[lane.ID] = lane

				for _, barrierID := range []string{entryID, exitID} {
					if existing, exists := barrierLane[barrierID]; exists {
						return nil, nil, nil, fmt.Errorf(
							"шлагбаум %q уже привязан к линии %q, повторно у линии %q",
							barrierID,
							existing.ID,
							lane.ID,
						)
					}

					barrierLane[barrierID] = lane
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
							key, existing.Lane.ID, lane.ID,
						)
					}
					triggerIndex[key] = lanes.TriggerTarget{
						Lane:      lane,
						Direction: device.Direction,
					}
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

func collectSources(
	laneConfig config.LaneConfig,
	deviceByID map[string]config.DeviceConfig,
) []*devices.Source {
	var sources []*devices.Source

	for _, deviceID := range laneConfig.Devices {
		device, ok := deviceByID[deviceID]
		if !ok {
			continue
		}

		switch device.Type {
		case config.DeviceTypeCamera,
			config.DeviceTypeKeypad,
			config.DeviceTypeCard:
			sources = append(sources, devices.NewSource(
				device.ID,
				device.Name,
				device.Type,
				device.ExternalID,
				device.Service,
				device.Direction,
			))
		}
	}

	return sources
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

func buildAxxonClients(
	services []config.ServiceConfig,
) (map[string]*axxon.Client, error) {
	clients := make(
		map[string]*axxon.Client,
	)

	for _, service := range services {
		if service.Type != config.ServiceTypeAxxon {
			continue
		}

		switch service.Mode {
		case config.ServiceModeDisabled:
			log.Printf("[axxon %s] отключён", service.ID)
			continue
		case config.ServiceModeStub:
			client, err := axxon.NewStub(service.PhotoFile, service.VideoFile)
			if err != nil {
				return nil, fmt.Errorf("создание тестового Axxon %q: %w", service.ID, err)
			}
			clients[service.ID] = client
			log.Printf("[axxon %s] stub: JPEG из %s", service.ID, service.PhotoFile)
			continue
		}

		client, err := axxon.New(
			axxon.Config{
				Address:      service.Address,
				VideoAddress: service.VideoAddress,
				Server:       service.Server,
				Username:     service.Username,
				Password:     service.Password,
				Timeout:      service.Timeout,
			},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"создание Axxon-клиента %q: %w",
				service.ID,
				err,
			)
		}

		clients[service.ID] = client
	}

	return clients, nil
}

func bindVideoRecorder(lane *lanes.Lane, cfg config.LaneConfig, clients map[string]*axxon.Client, services []config.ServiceConfig) error {
	if cfg.VideoService == "" && cfg.VideoCameraID == "" {
		return nil
	}
	if cfg.VideoService == "" || cfg.VideoCameraID == "" {
		return fmt.Errorf("линия %q: video_service и video_camera_id задаются вместе", cfg.ID)
	}
	var svc *config.ServiceConfig
	for i := range services {
		if services[i].ID == cfg.VideoService {
			svc = &services[i]
			break
		}
	}
	if svc == nil || svc.Type != config.ServiceTypeAxxon {
		return fmt.Errorf("линия %q: video_service %q не найден или не Axxon", cfg.ID, cfg.VideoService)
	}
	if svc.Mode == config.ServiceModeDisabled {
		return nil
	}
	if (svc.Mode == "" || svc.Mode == config.ServiceModeReal) && svc.VideoAddress == "" {
		return fmt.Errorf("линия %q: video_address обязателен для real-записи", cfg.ID)
	}
	if svc.Mode == config.ServiceModeStub && svc.VideoFile == "" {
		return fmt.Errorf("линия %q: video_file обязателен для stub-записи", cfg.ID)
	}
	if svc.VideoDuration <= 0 || svc.VideoStartDelay < 0 {
		return fmt.Errorf("линия %q: видео требует video_duration > 0 и video_start_delay >= 0", cfg.ID)
	}
	recorder, err := video.New(clients[svc.ID], cfg.VideoCameraID, svc.VideoStartDelay, svc.VideoDuration)
	if err != nil {
		return fmt.Errorf("линия %q: создание видеорекордера: %w", cfg.ID, err)
	}
	lane.SetVideoRecorder(recorder)
	return nil
}
