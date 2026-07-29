package modbus

import (
	"fmt"
	"log"
	"time"

	"github.com/simonvetter/modbus"

	"AutoGo/internal/emulator"
)

type Server struct {
	address  string
	unitID   uint8
	emulator *emulator.Emulator
}

type requestHandler struct {
	unitID uint8
	store  *emulator.DataStore
}

func NewServer(
	address string,
	unitID uint8,
	emulator *emulator.Emulator,
) *Server {
	return &Server{
		address:  address,
		unitID:   unitID,
		emulator: emulator,
	}
}

func (s *Server) Start() error {
	handler := &requestHandler{
		unitID: s.unitID,
		store:  s.emulator.Store(),
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
		"Modbus TCP Server запущен: %s, Unit ID: %d",
		s.address,
		s.unitID,
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
	if err := h.validateUnitID(req.UnitId); err != nil {
		return nil, err
	}

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
	if err := h.validateUnitID(req.UnitId); err != nil {
		return nil, err
	}

	return nil, modbus.ErrIllegalFunction
}

func (h *requestHandler) HandleDiscreteInputs(
	req *modbus.DiscreteInputsRequest,
) ([]bool, error) {
	if err := h.validateUnitID(req.UnitId); err != nil {
		return nil, err
	}

	return nil, modbus.ErrIllegalFunction
}

func (h *requestHandler) HandleInputRegisters(
	req *modbus.InputRegistersRequest,
) ([]uint16, error) {
	if err := h.validateUnitID(req.UnitId); err != nil {
		return nil, err
	}

	return nil, modbus.ErrIllegalFunction
}

func (h *requestHandler) validateUnitID(
	unitID uint8,
) error {
	if unitID != h.unitID {
		return modbus.ErrBadUnitId
	}

	return nil
}
