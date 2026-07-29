package plcclient

import (
	"fmt"

	"AutoGo/internal/plc"
)

type Status struct {
	Mode          plc.Mode
	State         plc.State
	ActualState   plc.State
	Alarm         plc.Alarm
	Warning       uint16
	Locked        bool
	LastCommand   plc.Command
	OperationTime float32
	WaitingTime   float32
	Attempts      uint16
	AttemptPeriod float32
}

func (c *Client) Status() (Status, error) {
	registers, err := c.readRegisters(
		0,
		plc.RegisterCount,
	)
	if err != nil {
		return Status{}, fmt.Errorf(
			"чтение состояния PLC: %w",
			err,
		)
	}

	if len(registers) != int(plc.RegisterCount) {
		return Status{}, fmt.Errorf(
			"PLC вернул неверное количество регистров: получено %d, ожидалось %d",
			len(registers),
			plc.RegisterCount,
		)
	}

	status := Status{
		Mode: plc.Mode(
			decodeInt32(registers, plc.RegisterOutMode),
		),

		State: plc.State(
			decodeInt32(registers, plc.RegisterOutState),
		),

		ActualState: plc.State(
			decodeInt32(registers, plc.RegisterOutStateActual),
		),

		Alarm: plc.Alarm(
			registers[plc.RegisterOutAlarm],
		),

		Warning: registers[plc.RegisterOutWarning],

		Locked: decodeInt32(
			registers,
			plc.RegisterOutLocked,
		) != 0,

		LastCommand: plc.Command(
			decodeInt32(registers, plc.RegisterOutCommand),
		),

		OperationTime: decodeFloat32(
			registers,
			plc.RegisterOutOperationTime,
		),

		WaitingTime: decodeFloat32(
			registers,
			plc.RegisterTimeWaiting,
		),

		Attempts: registers[plc.RegisterNumAttempts],

		AttemptPeriod: decodeFloat32(
			registers,
			plc.RegisterPeriodAttempts,
		),
	}

	return status, nil
}

func decodeInt32(
	registers []uint16,
	address uint16,
) int32 {
	return plc.DecodeInt32(
		registers[address],
		registers[address+1],
	)
}

func decodeFloat32(
	registers []uint16,
	address uint16,
) float32 {
	return plc.DecodeFloat32(
		registers[address],
		registers[address+1],
	)
}

func (s Status) HasAlarm() bool {
	return s.Alarm != plc.AlarmNone
}

func (s Status) HasAlarmFlag(alarm plc.Alarm) bool {
	if alarm == plc.AlarmNone {
		return s.Alarm == plc.AlarmNone
	}

	return s.Alarm&alarm != 0
}
