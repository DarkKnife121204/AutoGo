package main

import (
	"fmt"
	"log"
	"time"

	modbus "github.com/adibhanna/modbus-go"
	modbusTypes "github.com/adibhanna/modbus-go/modbus"

	"AutoGo/internal/plc"
)

const (
	plcAddress = "127.0.0.1:5020"
	unitID     = 1
)

func main() {
	readAll()

	writeCommand(plc.CommandStart)

	for i := 0; i < 12; i++ {
		state, err := readState()
		if err != nil {
			log.Printf("чтение состояния: %v", err)
		} else {
			fmt.Printf("State: %s (%d)\n", state, state)
		}

		time.Sleep(time.Second)
	}
}

func newClient() (*modbus.Client, error) {
	client := modbus.NewTCPClient(plcAddress)
	client.SetSlaveID(unitID)
	client.SetTimeout(3 * time.Second)
	client.SetRetryCount(1)

	if err := client.Connect(); err != nil {
		return nil, err
	}

	return client, nil
}

func readAll() {
	client, err := newClient()
	if err != nil {
		log.Fatalf("подключение: %v", err)
	}
	defer client.Close()

	registers, err := client.ReadHoldingRegisters(
		modbusTypes.Address(0),
		modbusTypes.Quantity(plc.RegisterCount),
	)
	if err != nil {
		log.Fatalf("чтение регистров: %v", err)
	}

	for address, value := range registers {
		fmt.Printf("HR%-2d = %d\n", address, value)
	}
}

func writeCommand(command plc.Command) {
	client, err := newClient()
	if err != nil {
		log.Fatalf("подключение: %v", err)
	}
	defer client.Close()

	registers := plc.EncodeInt32(int32(command))

	err = client.WriteMultipleRegisters(
		modbusTypes.Address(plc.RegisterInCommand),
		registers[:],
	)
	if err != nil {
		log.Fatalf("запись команды: %v", err)
	}

	fmt.Printf("Команда отправлена: %s\n", command)
}

func readState() (plc.State, error) {
	client, err := newClient()
	if err != nil {
		return plc.StateUnknown, err
	}
	defer client.Close()

	registers, err := client.ReadHoldingRegisters(
		modbusTypes.Address(plc.RegisterOutState),
		modbusTypes.Quantity(2),
	)
	if err != nil {
		return plc.StateUnknown, err
	}

	return plc.State(
		plc.DecodeInt32(registers[0], registers[1]),
	), nil
}
