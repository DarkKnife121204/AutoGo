package plcclient

import (
	"fmt"

	"AutoGo/internal/plc"
)

func (c *Client) Start() error {
	return c.sendCommand(plc.CommandStart)
}

func (c *Client) StartReverse() error {
	return c.sendCommand(plc.CommandStartReverse)
}

func (c *Client) Stop() error {
	return c.sendCommand(plc.CommandStop)
}

func (c *Client) Open() error {
	return c.sendCommand(plc.CommandOpen)
}

func (c *Client) CloseBarrier() error {
	return c.sendCommand(plc.CommandClose)
}

func (c *Client) Reset() error {
	return c.sendCommand(plc.CommandReset)
}

func (c *Client) sendCommand(command plc.Command) error {
	if command == plc.CommandNone {
		return fmt.Errorf("нельзя отправить пустую PLC-команду")
	}

	registers := plc.EncodeInt32(int32(command))

	if err := c.writeRegisters(
		plc.RegisterInCommand,
		registers[:],
	); err != nil {
		return fmt.Errorf(
			"отправка PLC-команды %s: %w",
			command,
			err,
		)
	}

	return nil
}
