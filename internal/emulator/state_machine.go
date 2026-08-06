package emulator

import (
	"log"
	"time"

	"AutoGo/internal/plc"
)

func (e *Emulator) initState() {
	time.Sleep(500 * time.Millisecond)

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != plc.StateInit {
		return
	}

	if err := e.setState(plc.StateClosed); err != nil {
		log.Printf("ошибка перехода в Closed: %v", err)
		return
	}

	if err := e.setActualState(plc.ActualStateClosed); err != nil {
		log.Printf("ошибка установки ActualState Closed: %v", err)
	}
}

func (e *Emulator) open() {
	operationID, ok := e.beginOperation(
		plc.StateClosed,
		plc.StateStopped,
	)
	if !ok {
		return
	}

	go func() {
		if !e.transition(
			operationID,
			plc.StateOpening,
			plc.ActualStateIntermediate,
			e.openDuration,
		) {
			return
		}

		e.finishOperation(
			operationID,
			plc.StateOpened,
			plc.ActualStateOpened,
		)
	}()
}

func (e *Emulator) close() {
	operationID, ok := e.beginOperation(
		plc.StateOpened,
		plc.StateStopped,
	)
	if !ok {
		return
	}

	go func() {
		if !e.transition(
			operationID,
			plc.StateClosing,
			plc.ActualStateIntermediate,
			e.closeDuration,
		) {
			return
		}

		e.finishOperation(
			operationID,
			plc.StateClosed,
			plc.ActualStateClosed,
		)
	}()
}

func (e *Emulator) startCycle() {
	operationID, ok := e.beginOperation(plc.StateClosed)
	if !ok {
		return
	}

	go func() {
		if !e.transition(
			operationID,
			plc.StateOpening,
			plc.ActualStateIntermediate,
			e.openDuration,
		) {
			return
		}

		if !e.transition(
			operationID,
			plc.StateOpened,
			plc.ActualStateOpened,
			e.transferDuration,
		) {
			return
		}

		if !e.transition(
			operationID,
			plc.StateClosing,
			plc.ActualStateIntermediate,
			e.closeDuration,
		) {
			return
		}

		e.finishOperation(
			operationID,
			plc.StateClosed,
			plc.ActualStateClosed,
		)
	}()
}

func (e *Emulator) startReverseCycle() {
	// Пока физическая эмуляция совпадает с прямым циклом.
	// Разница появится после добавления датчиков направления.
	e.startCycle()
}

func (e *Emulator) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.operationID++

	if err := e.setState(plc.StateStopped); err != nil {
		log.Printf("ошибка остановки: %v", err)
		return
	}

	switch e.actualState {
	case plc.ActualStateClosed, plc.ActualStateOpened:
	default:
		if err := e.setActualState(
			plc.ActualStateIntermediate,
		); err != nil {
			log.Printf(
				"ошибка установки промежуточного положения: %v",
				err,
			)
		}
	}

	log.Println("PLC: операция остановлена")
}

func (e *Emulator) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.operationID++

	if err := e.store.WriteUint16(
		plc.RegisterOutAlarm,
		uint16(plc.AlarmNone),
	); err != nil {
		log.Printf("ошибка сброса аварии: %v", err)
		return
	}

	if err := e.setState(plc.StateInit); err != nil {
		log.Printf("ошибка перехода в Init: %v", err)
		return
	}

	go e.determineStateAfterReset()
}

func (e *Emulator) determineStateAfterReset() {
	time.Sleep(500 * time.Millisecond)

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != plc.StateInit {
		return
	}

	switch e.actualState {
	case plc.ActualStateClosed:
		if err := e.setState(plc.StateClosed); err != nil {
			log.Printf("ошибка определения Closed: %v", err)
		}

	case plc.ActualStateOpened:
		if err := e.setState(plc.StateOpened); err != nil {
			log.Printf("ошибка определения Opened: %v", err)
		}

	case plc.ActualStateInvalid:
		if err := e.store.WriteUint16(
			plc.RegisterOutAlarm,
			uint16(plc.AlarmLimitConflict),
		); err != nil {
			log.Printf("ошибка записи аварии: %v", err)
			return
		}

		if err := e.setState(plc.StateAlarm); err != nil {
			log.Printf("ошибка перехода в Alarm: %v", err)
		}

	default:
		if err := e.setState(plc.StateStopped); err != nil {
			log.Printf("ошибка определения Stopped: %v", err)
		}
	}
}

func (e *Emulator) beginOperation(
	allowedStates ...plc.State,
) (uint64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !stateAllowed(e.state, allowedStates) {
		log.Printf(
			"PLC: команда запрещена в состоянии %s",
			e.state,
		)

		return 0, false
	}

	e.operationID++
	e.operationStarted = time.Now()

	operationID := e.operationID

	if err := e.store.WriteFloat32(
		plc.RegisterOutOperationTime,
		0,
	); err != nil {
		log.Printf("ошибка сброса времени операции: %v", err)
	}

	go e.trackOperationTime(operationID)

	return operationID, true
}

func stateAllowed(
	current plc.State,
	allowed []plc.State,
) bool {
	for _, state := range allowed {
		if current == state {
			return true
		}
	}

	return false
}

func (e *Emulator) trackOperationTime(operationID uint64) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		e.mu.Lock()

		if operationID != e.operationID {
			e.mu.Unlock()
			return
		}

		elapsed := time.Since(
			e.operationStarted,
		).Seconds()

		e.mu.Unlock()

		if err := e.store.WriteFloat32(
			plc.RegisterOutOperationTime,
			float32(elapsed),
		); err != nil {
			log.Printf(
				"ошибка записи времени операции: %v",
				err,
			)
			return
		}
	}
}

func (e *Emulator) transition(
	operationID uint64,
	state plc.State,
	actualState plc.ActualState,
	duration time.Duration,
) bool {
	e.mu.Lock()

	if operationID != e.operationID {
		e.mu.Unlock()
		return false
	}

	if err := e.setState(state); err != nil {
		e.mu.Unlock()
		log.Printf("ошибка смены состояния: %v", err)
		return false
	}

	if err := e.setActualState(actualState); err != nil {
		e.mu.Unlock()
		log.Printf(
			"ошибка смены фактического состояния: %v",
			err,
		)
		return false
	}

	e.mu.Unlock()

	log.Printf(
		"PLC: State=%s ActualState=%s",
		state,
		actualState,
	)

	timer := time.NewTimer(duration)
	defer timer.Stop()

	<-timer.C

	e.mu.Lock()
	active := operationID == e.operationID
	e.mu.Unlock()

	return active
}

func (e *Emulator) finishOperation(
	operationID uint64,
	state plc.State,
	actualState plc.ActualState,
) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if operationID != e.operationID {
		return
	}

	elapsed := time.Since(
		e.operationStarted,
	).Seconds()

	if err := e.store.WriteFloat32(
		plc.RegisterOutOperationTime,
		float32(elapsed),
	); err != nil {
		log.Printf("ошибка записи времени операции: %v", err)
	}

	e.operationID++

	if err := e.setState(state); err != nil {
		log.Printf("ошибка завершения операции: %v", err)
		return
	}

	if err := e.setActualState(actualState); err != nil {
		log.Printf(
			"ошибка установки фактического состояния: %v",
			err,
		)
		return
	}

	log.Printf(
		"PLC: State=%s ActualState=%s",
		state,
		actualState,
	)
}

func (e *Emulator) setState(state plc.State) error {
	e.state = state

	return e.store.WriteInt32(
		plc.RegisterOutState,
		int32(state),
	)
}

func (e *Emulator) setActualState(
	state plc.ActualState,
) error {
	e.actualState = state

	return e.store.WriteInt32(
		plc.RegisterOutStateActual,
		int32(state),
	)
}
