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

	if err := e.setState(plc.StateWaiting); err != nil {
		log.Printf("ошибка перехода в Waiting: %v", err)
	}
}

func (e *Emulator) open() {
	operationID, ok := e.beginOperation()
	if !ok {
		return
	}

	go func() {
		if !e.transition(
			operationID,
			plc.StateOpeningBarrier,
			e.openDuration,
		) {
			return
		}

		e.finishOperation(
			operationID,
			plc.StateWaitingTransfer,
		)
	}()
}

func (e *Emulator) close() {
	e.mu.Lock()

	if e.state != plc.StateWaitingTransfer &&
		e.state != plc.StateWaiting {
		log.Printf(
			"PLC: Close запрещена в состоянии %s",
			e.state,
		)
		e.mu.Unlock()
		return
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

	e.mu.Unlock()

	go func() {
		if !e.transition(
			operationID,
			plc.StateClosingBarrier,
			e.closeDuration,
		) {
			return
		}

		e.finishOperation(
			operationID,
			plc.StateWaiting,
		)
	}()
}

func (e *Emulator) startCycle() {
	operationID, ok := e.beginOperation()
	if !ok {
		return
	}

	go func() {
		if !e.transition(
			operationID,
			plc.StateOpeningBarrier,
			e.openDuration,
		) {
			return
		}

		if !e.transition(
			operationID,
			plc.StateWaitingTransfer,
			e.transferDuration,
		) {
			return
		}

		if !e.transition(
			operationID,
			plc.StateClosingBarrier,
			e.closeDuration,
		) {
			return
		}

		e.finishOperation(
			operationID,
			plc.StateWaiting,
		)
	}()
}

func (e *Emulator) startReverseCycle() {
	// Пока как Start
	e.startCycle()
}

func (e *Emulator) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.operationID++

	if err := e.setState(plc.StateWaiting); err != nil {
		log.Printf("ошибка остановки: %v", err)
		return
	}

	log.Println("PLC: операция остановлена")
}

func (e *Emulator) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != plc.StateAlarm {
		log.Println("PLC: Reset проигнорирован — аварии нет")
		return
	}

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

	go e.initState()
}

func (e *Emulator) beginOperation() (uint64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != plc.StateWaiting {
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

func (e *Emulator) trackOperationTime(operationID uint64) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		e.mu.Lock()

		if operationID != e.operationID {
			e.mu.Unlock()
			return
		}

		elapsed := time.Since(e.operationStarted).Seconds()

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

func (e *Emulator) transition(operationID uint64, state plc.State, duration time.Duration) bool {
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

	e.mu.Unlock()

	log.Printf("PLC: состояние %s", state)

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
) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if operationID != e.operationID {
		return
	}

	elapsed := time.Since(e.operationStarted).Seconds()

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

	log.Printf("PLC: состояние %s", state)
}

func (e *Emulator) setState(state plc.State) error {
	e.state = state

	if err := e.store.WriteInt32(
		plc.RegisterOutState,
		int32(state),
	); err != nil {
		return err
	}

	return e.store.WriteInt32(
		plc.RegisterOutStateActual,
		int32(state),
	)
}
