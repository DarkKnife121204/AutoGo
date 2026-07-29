package plcclient

import (
	"fmt"

	"AutoGo/internal/plc"
)

// Registers возвращает полную карту Holding Registers
func (c *Client) Registers() ([]uint16, error) {
	registers, err := c.readRegisters(
		0,
		plc.RegisterCount,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"чтение регистров PLC: %w",
			err,
		)
	}

	return registers, nil
}
