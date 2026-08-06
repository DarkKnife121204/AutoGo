package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ControllerTypeModbusTCP = "modbus_tcp"

	DeviceTypeBarrier      = "barrier"
	DeviceTypeTrafficLight = "traffic_light"
	DeviceTypeDisplay      = "display"
	DeviceTypeCamera       = "camera"
	DeviceTypeSensor       = "sensor"

	ScenarioTypeSingleBarrier = "single_barrier"
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
	case ControllerTypeModbusTCP:
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

	return errors.Join(validationErrors...)
}

func validateLane(
	path string,
	lane LaneConfig,
	deviceIDs map[string]struct{},
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
) error {
	var validationErrors []error

	switch scenario.Type {
	case ScenarioTypeSingleBarrier:
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

	barrierID := strings.TrimSpace(
		scenario.Settings["barrier"],
	)

	if barrierID == "" {
		validationErrors = append(
			validationErrors,
			fmt.Errorf(
				"%s.settings.barrier обязателен",
				path,
			),
		)

		return errors.Join(validationErrors...)
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
	}

	return errors.Join(validationErrors...)
}
