package modbus

import (
	"fmt"
	"log"

	"github.com/adibhanna/modbus-go"

	"AutoGo/internal/emulator"
)

type Server struct {
	address  string
	emulator *emulator.Emulator
}

func NewServer(address string, emulator *emulator.Emulator) *Server {
	return &Server{
		address:  address,
		emulator: emulator,
	}
}

func (s *Server) Start() error {
	server, err := modbus.NewTCPServer(
		s.address,
		s.emulator.Store().ModbusStore(),
	)
	if err != nil {
		return fmt.Errorf("создание Modbus TCP Server: %w", err)
	}

	s.emulator.Start()

	log.Printf("Modbus TCP Server запущен: %s", s.address)

	if err := server.Start(); err != nil {
		return fmt.Errorf("запуск Modbus TCP Server: %w", err)
	}

	select {}

}
