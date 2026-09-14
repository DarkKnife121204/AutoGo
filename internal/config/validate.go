package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ControllerTypeSiemens = "siemens"

	DeviceTypeBarrier      = "barrier"
	DeviceTypeTrafficLight = "traffic_light"
	DeviceTypeDisplay      = "display"
	DeviceTypeCamera       = "camera"
	DeviceTypeKeypad       = "keypad"
	DeviceTypeCard         = "card"
	DeviceTypeSensor       = "sensor"

	ScenarioTypeSingleBarrier = "single_barrier"
	ScenarioTypeDoubleBarrier = "double_barrier"

	ReleaseModeImmediate            = "immediate"
	ReleaseModeExternalConfirmation = "external_confirmation"

	DirectionNormal  = "normal"
	DirectionReverse = "reverse"

	LaneModeAutomatic = "automatic"
	LaneModeManual    = "manual"
)

func (c Config) Validate() error {
	var validationErrors []error

	if c.Version != 1 {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"неподдерживаемая версия конфигурации: %d",
				c.Version,
			),
		)
	}

	if strings.TrimSpace(c.Site.ID) == "" {
		validationErrors = append(
			validationErrors,
			errors.New("site.id обязателен"),
		)
	}

	if strings.TrimSpace(c.Site.Name) == "" {
		validationErrors = append(
			validationErrors,
			errors.New("site.name обязателен"),
		)
	}

	controllerIDs := make(map[string]struct{})
	deviceIDs := make(map[string]struct{})
	deviceTypes := make(map[string]string)
	checkpointIDs := make(map[string]struct{})
	laneIDs := make(map[string]struct{})

	for index, controller := range c.Controllers {
		path := fmt.Sprintf("controllers[%d]", index)

		if err := validateID(
			path+".id",
			controller.ID,
			controllerIDs,
		); err != nil {
			validationErrors = append(
				validationErrors,
				err,
			)
		}

		if err := validateController(
			path,
			controller,
		); err != nil {
			validationErrors = append(
				validationErrors,
				err,
			)
		}
	}

	for index, device := range c.Devices {
		path := fmt.Sprintf("devices[%d]", index)

		if err := validateID(
			path+".id",
			device.ID,
			deviceIDs,
		); err != nil {
			validationErrors = append(
				validationErrors,
				err,
			)
		}

		if err := validateDevice(
			path,
			device,
			controllerIDs,
		); err != nil {
			validationErrors = append(
				validationErrors,
				err,
			)
		}
		deviceTypes[strings.TrimSpace(device.ID)] = device.Type
	}

	for checkpointIndex, checkpoint := range c.Checkpoints {
		checkpointPath := fmt.Sprintf(
			"checkpoints[%d]",
			checkpointIndex,
		)

		if err := validateID(
			checkpointPath+".id",
			checkpoint.ID,
			checkpointIDs,
		); err != nil {
			validationErrors = append(
				validationErrors,
				err,
			)
		}

		if strings.TrimSpace(checkpoint.Name) == "" {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.name обязателен",
					checkpointPath,
				),
			)
		}

		for laneIndex, lane := range checkpoint.Lanes {
			lanePath := fmt.Sprintf(
				"%s.lanes[%d]",
				checkpointPath,
				laneIndex,
			)

			if err := validateID(
				lanePath+".id",
				lane.ID,
				laneIDs,
			); err != nil {
				validationErrors = append(
					validationErrors,
					err,
				)
			}

			if err := validateLane(
				lanePath,
				lane,
				deviceIDs,
				deviceTypes,
			); err != nil {
				validationErrors = append(
					validationErrors,
					err,
				)
			}
		}
	}

	return errors.Join(validationErrors...)
}

func validateID(
	path string,
	id string,
	ids map[string]struct{},
) error {
	id = strings.TrimSpace(id)

	if id == "" {
		return fmt.Errorf("%s обязателен", path)
	}

	if _, exists := ids[id]; exists {
		return fmt.Errorf(
			"%s содержит повторяющийся ID %q",
			path,
			id,
		)
	}

	ids[id] = struct{}{}

	return nil
}

func validateController(
	path string,
	controller ControllerConfig,
) error {
	var validationErrors []error

	switch controller.Type {
	case ControllerTypeSiemens:
	default:
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.type содержит неизвестный тип %q",
				path,
				controller.Type,
			),
		)
	}

	if strings.TrimSpace(controller.Address) == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.address обязателен",
				path,
			),
		)
	}

	if controller.UnitID == 0 {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.unit_id должен быть больше 0",
				path,
			),
		)
	}

	if controller.Timeout <= 0 {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.timeout должен быть больше 0",
				path,
			),
		)
	}

	return errors.Join(validationErrors...)
}

func validateDevice(
	path string,
	device DeviceConfig,
	controllerIDs map[string]struct{},
) error {
	var validationErrors []error

	switch device.Type {
	case DeviceTypeBarrier,
		DeviceTypeTrafficLight,
		DeviceTypeDisplay,
		DeviceTypeCamera,
		DeviceTypeKeypad,
		DeviceTypeCard,
		DeviceTypeSensor:

	default:
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.type содержит неизвестный тип %q",
				path,
				device.Type,
			),
		)
	}

	if strings.TrimSpace(device.Name) == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.name обязателен",
				path,
			),
		)
	}

	if device.Type == DeviceTypeBarrier {
		if strings.TrimSpace(device.Controller) == "" {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.controller обязателен для barrier",
					path,
				),
			)
		} else if _, exists := controllerIDs[device.Controller]; !exists {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.controller ссылается на неизвестный controller %q",
					path,
					device.Controller,
				),
			)
		}
	}

	switch device.Type {
	case DeviceTypeCamera, DeviceTypeKeypad, DeviceTypeCard:
		if strings.TrimSpace(device.ExternalID) == "" {
			validationErrors = append(
				validationErrors,
				fmt.Errorf("%s.external_id обязателен для %s", path, device.Type),
			)
		}

		switch strings.TrimSpace(device.Direction) {
		case "", DirectionNormal, DirectionReverse:
		default:
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.direction содержит неизвестное значение %q",
					path, device.Direction,
				),
			)
		}

		if strings.TrimSpace(device.Controller) != "" {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.controller недопустим для %s",
					path,
					device.Type,
				),
			)
		}
	}

	return errors.Join(validationErrors...)
}

func validateLane(
	path string,
	lane LaneConfig,
	deviceIDs map[string]struct{},
	deviceTypes map[string]string,
) error {
	var validationErrors []error

	if strings.TrimSpace(lane.Name) == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.name обязателен",
				path,
			),
		)
	}

	laneDeviceIDs := make(map[string]struct{})

	switch strings.TrimSpace(lane.Mode) {
	case "", LaneModeAutomatic, LaneModeManual:
	default:
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.mode содержит неизвестный режим %q (ожидается automatic или manual)",
				path,
				lane.Mode,
			),
		)
	}

	for index, deviceID := range lane.Devices {
		devicePath := fmt.Sprintf(
			"%s.devices[%d]",
			path,
			index,
		)

		deviceID = strings.TrimSpace(deviceID)

		if deviceID == "" {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s не может быть пустым",
					devicePath,
				),
			)

			continue
		}

		if _, exists := deviceIDs[deviceID]; !exists {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s ссылается на неизвестное устройство %q",
					devicePath,
					deviceID,
				),
			)

			continue
		}

		if _, exists := laneDeviceIDs[deviceID]; exists {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s повторяет устройство %q",
					devicePath,
					deviceID,
				),
			)

			continue
		}

		laneDeviceIDs[deviceID] = struct{}{}
	}

	if err := validateScenario(
		path+".scenario",
		lane.Scenario,
		laneDeviceIDs,
		deviceTypes,
	); err != nil {
		validationErrors = append(
			validationErrors,
			err,
		)
	}

	return errors.Join(validationErrors...)
}

func validateScenario(
	path string,
	scenario ScenarioConfig,
	laneDeviceIDs map[string]struct{},
	deviceTypes map[string]string,
) error {
	var validationErrors []error

	switch scenario.Type {
	case ScenarioTypeSingleBarrier:
		validationErrors = append(
			validationErrors,
			validateSingleBarrierSettings(
				path,
				scenario.Settings,
				laneDeviceIDs,
				deviceTypes,
			)...,
		)

	case ScenarioTypeDoubleBarrier:
		validationErrors = append(
			validationErrors,
			validateDoubleBarrierSettings(
				path,
				scenario.Settings,
				laneDeviceIDs,
				deviceTypes,
			)...,
		)

	default:
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.type содержит неизвестный сценарий %q",
				path,
				scenario.Type,
			),
		)

		return errors.Join(validationErrors...)
	}

	validationErrors = append(
		validationErrors,
		validateReleaseMode(path, scenario.Settings.ReleaseMode)...,
	)

	if scenario.Settings.QueueDepth < 0 {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.queue_depth не может быть отрицательным",
				path,
			),
		)
	}

	return errors.Join(validationErrors...)
}

func validateSingleBarrierSettings(
	path string,
	settings ScenarioSettings,
	laneDeviceIDs map[string]struct{},
	deviceTypes map[string]string,
) []error {
	var validationErrors []error

	barrierID := strings.TrimSpace(settings.Barrier)

	if barrierID == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.barrier обязателен",
				path,
			),
		)

		return validationErrors
	}

	if _, exists := laneDeviceIDs[barrierID]; !exists {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.barrier ссылается на устройство %q, отсутствующее в линии",
				path,
				barrierID,
			),
		)

		return validationErrors
	}

	if deviceTypes[barrierID] != DeviceTypeBarrier {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.barrier должен ссылаться на устройство типа barrier, %q имеет тип %q",
				path,
				barrierID,
				deviceTypes[barrierID],
			),
		)
	}

	return validationErrors
}

func validateDoubleBarrierSettings(
	path string,
	settings ScenarioSettings,
	laneDeviceIDs map[string]struct{},
	deviceTypes map[string]string,
) []error {
	var validationErrors []error

	entry := strings.TrimSpace(settings.EntryBarrier)
	exit := strings.TrimSpace(settings.ExitBarrier)

	if entry == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf("%s.settings.entry_barrier обязателен", path),
		)
	} else if _, ok := laneDeviceIDs[entry]; !ok {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.entry_barrier ссылается на устройство %q, отсутствующее в линии",
				path, entry,
			),
		)
	}

	if exit == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf("%s.settings.exit_barrier обязателен", path),
		)
	} else if _, ok := laneDeviceIDs[exit]; !ok {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.exit_barrier ссылается на устройство %q, отсутствующее в линии",
				path, exit,
			),
		)
	}

	if entry != "" && entry == exit {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings: entry_barrier и exit_barrier должны быть разными",
				path,
			),
		)
	}

	if entry != "" {
		if _, exists := laneDeviceIDs[entry]; exists &&
			deviceTypes[entry] != DeviceTypeBarrier {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.settings.entry_barrier должен ссылаться на устройство типа barrier, %q имеет тип %q",
					path,
					entry,
					deviceTypes[entry],
				),
			)
		}
	}

	if exit != "" {
		if _, exists := laneDeviceIDs[exit]; exists &&
			deviceTypes[exit] != DeviceTypeBarrier {
			validationErrors = append(
				validationErrors,
				fmt.Errorf(
					"%s.settings.exit_barrier должен ссылаться на устройство типа barrier, %q имеет тип %q",
					path,
					exit,
					deviceTypes[exit],
				),
			)
		}
	}

	return validationErrors
}

func validateReleaseMode(
	path string,
	releaseMode string,
) []error {
	switch strings.TrimSpace(releaseMode) {
	case "", ReleaseModeImmediate, ReleaseModeExternalConfirmation:
		return nil

	default:
		return []error{
			fmt.Errorf(
				"%s.settings.release_mode содержит неизвестное значение %q",
				path,
				releaseMode,
			),
		}
	}
}
