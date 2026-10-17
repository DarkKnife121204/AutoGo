package emulator

import (
	"log"
	"time"

	"AutoGo/internal/plc"
)

func (e *Emulator) Start() {
	go e.commandLoop()
	go e.initState()
}

func (e *Emulator) commandLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		command := e.store.TakeCommand()

		if command == plc.CommandNone {
			continue
		}

		e.handleCommand(command)
	}
}

func (e *Emulator) handleCommand(command plc.Command) {
	log.Printf("PLC: получена команда %s", command)

	if err := e.store.WriteInt32(
		plc.RegisterOutCommand,
		int32(command),
	); err != nil {
		log.Printf("ошибка записи out_cmd: %v", err)
		return
	}

	switch command {
	case plc.CommandOpen:
		e.open()
	case plc.CommandStart:
		e.startCycle()
	case plc.CommandStartReverse:
		e.startReverseCycle()
	case plc.CommandStop:
		e.stop()
	case plc.CommandReset:
		e.reset()
	default:
		log.Printf("PLC: неизвестная команда %d", command)
	}
}
