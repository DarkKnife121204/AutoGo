package emulator

import (
	"fmt"
	"sync"
	"time"

	"AutoGo/internal/plc"
)

type Emulator struct {
	store            *DataStore
	mu               sync.Mutex
	operationID      uint64
	operationStarted time.Time
	state            plc.State
	openDuration     time.Duration
	transferDuration time.Duration
	closeDuration    time.Duration
}

func New(openDuration time.Duration, transferDuration time.Duration, closeDuration time.Duration) (*Emulator, error) {
	emulator := &Emulator{
		store:            NewDataStore(),
		state:            plc.StateInit,
		openDuration:     openDuration,
		transferDuration: transferDuration,
		closeDuration:    closeDuration,
	}

	if err := emulator.initialize(); err != nil {
		return nil, fmt.Errorf("инициализация эмулятора: %w", err)
	}

	return emulator, nil
}

func (e *Emulator) Store() *DataStore {
	return e.store
}

func (e *Emulator) initialize() error {
	if err := e.setState(plc.StateInit); err != nil {
		return err
	}

	if err := e.store.WriteInt32(
		plc.RegisterInCommand,
		int32(plc.CommandNone),
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterInConfig,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterInAlarm,
		uint16(plc.AlarmNone),
	); err != nil {
		return err
	}

	if err := e.store.WriteInt32(
		plc.RegisterOutMode,
		int32(plc.ModeAutomatic),
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterOutAlarm,
		uint16(plc.AlarmNone),
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterOutWarning,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteInt32(
		plc.RegisterOutLocked,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteInt32(
		plc.RegisterOutCommand,
		int32(plc.CommandNone),
	); err != nil {
		return err
	}

	if err := e.store.WriteFloat32(
		plc.RegisterOutOperationTime,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterInSAUCommand,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterInSAUCommandHi,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterInCommandLock,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterInModeLock,
		0,
	); err != nil {
		return err
	}

	if err := e.store.WriteFloat32(
		plc.RegisterTimeWaiting,
		float32(e.transferDuration.Seconds()),
	); err != nil {
		return err
	}

	if err := e.store.WriteUint16(
		plc.RegisterNumAttempts,
		3,
	); err != nil {
		return err
	}

	return e.store.WriteFloat32(
		plc.RegisterPeriodAttempts,
		1,
	)
}
