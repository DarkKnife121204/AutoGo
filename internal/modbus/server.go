package modbus

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/simonvetter/modbus"

	"AutoGo/internal/emulator"
)

type Server struct {
	address  string
	emulator *emulator.Emulator
}

type requestHandler struct {
	store *emulator.DataStore
}

func NewServer(
	address string,
	emulator *emulator.Emulator,
) *Server {
	return &Server{
		address:  address,
		emulator: emulator,
	}
}

func (s *Server) Start() error {
	handler := &requestHandler{
		store: s.emulator.Store(),
	}

	server, err := modbus.NewServer(
		&modbus.ServerConfiguration{
			URL:        "tcp://" + s.address,
			Timeout:    30 * time.Second,
			MaxClients: 10,
		},
		handler,
	)
	if err != nil {
		return fmt.Errorf(
			"создание Modbus TCP Server: %w",
			err,
		)
	}

	s.emulator.Start()

	log.Printf(
		"Modbus TCP Server запущен: %s",
		s.address,
	)

	if err := server.Start(); err != nil {
		return fmt.Errorf(
			"запуск Modbus TCP Server: %w",
			err,
		)
	}

	select {}
}

func (h *requestHandler) HandleHoldingRegisters(
	req *modbus.HoldingRegistersRequest,
) ([]uint16, error) {
	if req.IsWrite {
		if err := h.store.WriteRegisters(
			req.Addr,
			req.Args,
		); err != nil {
			return nil, err
		}

		return nil, nil
	}

	return h.store.ReadRegisters(
		req.Addr,
		req.Quantity,
	)
}

func (h *requestHandler) HandleCoils(
	req *modbus.CoilsRequest,
) ([]bool, error) {
	return nil, errors.New("coils не поддерживаются")
}

func (h *requestHandler) HandleDiscreteInputs(
	req *modbus.DiscreteInputsRequest,
) ([]bool, error) {
	return nil, errors.New(
		"discrete inputs не поддерживаются",
	)
}

func (h *requestHandler) HandleInputRegisters(
	req *modbus.InputRegistersRequest,
) ([]uint16, error) {
	return nil, errors.New(
		"input registers не поддерживаются",
	)
}
