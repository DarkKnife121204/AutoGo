package plcclient

import (
	"fmt"
	"time"

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
	if err := c.sendCommand(plc.CommandReset); err != nil {
		return err
	}

	return c.waitCommandAck(
		plc.CommandReset,
		5*time.Second,
	)
}

func (c *Client) waitCommandAck(
	command plc.Command,
	timeout time.Duration,
) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		registers, err := c.readRegisters(
			0,
			plc.RegisterOutCommand+2,
		)
		if err != nil {
			return fmt.Errorf(
				"ожидание подтверждения команды %s: %w",
				command,
				err,
			)
		}

		inCommand := plc.Command(
			plc.DecodeInt32(
				registers[plc.RegisterInCommand],
				registers[plc.RegisterInCommand+1],
			),
		)

		outCommand := plc.Command(
			plc.DecodeInt32(
				registers[plc.RegisterOutCommand],
				registers[plc.RegisterOutCommand+1],
			),
		)

		if inCommand == plc.CommandNone &&
			outCommand == command {
			return nil
		}

		select {
		case <-ticker.C:

		case <-timer.C:
			return fmt.Errorf(
				"таймаут подтверждения PLC-команды %s: in=%s out=%s",
				command,
				inCommand,
				outCommand,
			)
		}
	}
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
