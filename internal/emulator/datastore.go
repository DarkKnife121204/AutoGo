package emulator

import (
	"fmt"
	"sync"

	"AutoGo/internal/plc"
)

type DataStore struct {
	registers []uint16
	mu        sync.RWMutex
}

func NewDataStore() *DataStore {
	return &DataStore{
		registers: make([]uint16, plc.RegisterCount),
	}
}

func (s *DataStore) ReadRegisters(
	address uint16,
	quantity uint16,
) ([]uint16, error) {
	if err := validateRange(address, quantity); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]uint16, quantity)

	copy(
		result,
		s.registers[address:address+quantity],
	)

	return result, nil
}

func (s *DataStore) WriteRegisters(
	address uint16,
	values []uint16,
) error {
	if err := validateRange(
		address,
		uint16(len(values)),
	); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	copy(
		s.registers[address:address+uint16(len(values))],
		values,
	)

	return nil
}

func (s *DataStore) ReadInt32(address uint16) (int32, error) {
	registers, err := s.ReadRegisters(address, 2)
	if err != nil {
		return 0, err
	}

	return plc.DecodeInt32(
		registers[0],
		registers[1],
	), nil
}

func (s *DataStore) WriteInt32(
	address uint16,
	value int32,
) error {
	registers := plc.EncodeInt32(value)

	return s.WriteRegisters(
		address,
		registers[:],
	)
}

func (s *DataStore) ReadFloat32(
	address uint16,
) (float32, error) {
	registers, err := s.ReadRegisters(address, 2)
	if err != nil {
		return 0, err
	}

	return plc.DecodeFloat32(
		registers[0],
		registers[1],
	), nil
}

func (s *DataStore) WriteFloat32(
	address uint16,
	value float32,
) error {
	registers := plc.EncodeFloat32(value)

	return s.WriteRegisters(
		address,
		registers[:],
	)
}

func (s *DataStore) ReadUint16(
	address uint16,
) (uint16, error) {
	registers, err := s.ReadRegisters(address, 1)
	if err != nil {
		return 0, err
	}

	return registers[0], nil
}

func (s *DataStore) WriteUint16(
	address uint16,
	value uint16,
) error {
	return s.WriteRegisters(
		address,
		[]uint16{value},
	)
}

func validateRange(
	address uint16,
	quantity uint16,
) error {
	if quantity == 0 {
		return fmt.Errorf(
			"количество регистров не может быть равно 0",
		)
	}

	end := uint32(address) + uint32(quantity)

	if end > uint32(plc.RegisterCount) {
		return fmt.Errorf(
			"регистры вне диапазона: address=%d quantity=%d",
			address,
			quantity,
		)
	}

	return nil
}
