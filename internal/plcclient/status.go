package plcclient

import (
	"fmt"

	"AutoGo/internal/plc"
)

type Status struct {
	Mode          int32
	State         plc.State
	ActualState   int32
	Alarm         plc.Alarm
	Warning       int16
	Locked        int32
	OperationTime int32
	WaitingTime   float32
	Attempts      int16
	AttemptPeriod float32
	OutCommand    int32
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
		Mode: decodeInt32(
			registers,
			plc.RegisterOutMode,
		),

		State: plc.State(
			decodeInt32(registers, plc.RegisterOutState),
		),

		ActualState: decodeInt32(registers, plc.RegisterOutStateActual),

		Alarm: plc.Alarm(
			registers[plc.RegisterOutAlarm],
		),

		Warning: int16(registers[plc.RegisterOutWarning]),

		Locked: decodeInt32(
			registers,
			plc.RegisterOutLocked,
		),

		OutCommand: decodeInt32(
			registers,
			plc.RegisterOutCommand,
		),

		OperationTime: decodeInt32(
			registers,
			plc.RegisterOutOperationTime,
		),

		WaitingTime: decodeFloat32(
			registers,
			plc.RegisterTimeWaiting,
		),

		Attempts: int16(registers[plc.RegisterNumAttempts]),

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
	return s.Alarm != plc.AlarmNone &&
		s.Alarm != plc.AlarmNoEvent
}

func (s Status) HasAlarmFlag(alarm plc.Alarm) bool {
	return s.Alarm == alarm
}
