package devices

import (
	"fmt"
	"log"
	"sync"
	"time"

	"AutoGo/internal/plc"
	"AutoGo/internal/plcclient"
)

type Barrier struct {
	ID           string
	Name         string
	ControllerID string
	PLC          *plcclient.Client

	mu        sync.Mutex
	lastState string
}

func NewBarrier(
	id string,
	name string,
	controllerID string,
	plc *plcclient.Client,
) (*Barrier, error) {
	if plc == nil {
		return nil, fmt.Errorf(
			"для шлагбаума %q не указан PLC-клиент",
			id,
		)
	}

	return &Barrier{
		ID:           id,
		Name:         name,
		ControllerID: controllerID,
		PLC:          plc,
	}, nil
}

func (b *Barrier) Start() error {
	return b.PLC.Start()
}

func (b *Barrier) StartReverse() error {
	return b.PLC.StartReverse()
}

func (b *Barrier) Stop() error {
	return b.PLC.Stop()
}

func (b *Barrier) Open() error {
	return b.PLC.Open()
}

func (b *Barrier) Close() error {
	return b.PLC.CloseBarrier()
}

func (b *Barrier) Reset() error {
	return b.PLC.Reset()
}

func (b *Barrier) WaitReadyForStart() error {
	const (
		timeout      = 5 * time.Second
		pollInterval = 100 * time.Millisecond
	)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		status, err := b.Status()
		if err != nil {
			return fmt.Errorf(
				"проверка готовности шлагбаума %q: %w",
				b.ID,
				err,
			)
		}

		if status.HasAlarm() {
			return fmt.Errorf(
				"шлагбаум %q не готов: alarm=%d",
				b.ID,
				status.Alarm,
			)
		}

		if status.State == plc.StateClosed {
			return nil
		}

		select {
		case <-ticker.C:

		case <-timer.C:
			return fmt.Errorf(
				"таймаут ожидания готовности шлагбаума %q: state=%s",
				b.ID,
				status.State,
			)
		}
	}
}

func (b *Barrier) Status() (plcclient.Status, error) {
	status, err := b.PLC.Status()
	if err != nil {
		return status, err
	}

	b.logStateChange(status.State.String())

	return status, nil
}

func (b *Barrier) logStateChange(state string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state == b.lastState {
		return
	}

	prev := b.lastState
	if prev == "" {
		prev = "—"
	}

	b.lastState = state

	log.Printf(
		"[barrier %s] %s -> %s",
		b.ID, prev, state,
	)
}
