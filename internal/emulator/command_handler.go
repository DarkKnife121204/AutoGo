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
		commandValue, err := e.store.ReadInt32(plc.RegisterInCommand)
		if err != nil {
			log.Printf("ошибка чтения команды: %v", err)
			continue
		}

		command := plc.Command(commandValue)

		if command == plc.CommandNone {
			continue
		}

		if err := e.store.WriteInt32(
			plc.RegisterInCommand,
			int32(plc.CommandNone),
		); err != nil {
			log.Printf("ошибка очистки команды: %v", err)
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
	case plc.CommandClose:
		e.close()
	default:
		log.Printf("PLC: неизвестная команда %d", command)
	}
}
