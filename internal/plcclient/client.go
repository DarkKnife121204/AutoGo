package plcclient

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	modbus "github.com/simonvetter/modbus"
)

var ErrClosed = errors.New("PLC-клиент закрыт")

type Config struct {
	Address string
	UnitID  uint8
	Timeout time.Duration
}

type Client struct {
	config Config

	mu     sync.Mutex
	client *modbus.ModbusClient
	closed bool
}

func New(config Config) (*Client, error) {
	config.Address = strings.TrimSpace(config.Address)

	if config.Address == "" {
		return nil, errors.New("адрес PLC не указан")
	}

	if config.Timeout <= 0 {
		return nil, errors.New("таймаут PLC должен быть больше нуля")
	}

	return &Client{
		config: config,
	}, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if c.client == nil {
		return nil
	}

	err := c.client.Close()
	c.client = nil

	if err != nil {
		return fmt.Errorf("закрытие соединения с PLC: %w", err)
	}

	log.Printf("[plc %s] подключено", c.config.Address)

	return nil
}

func (c *Client) connectLocked() error {
	if c.closed {
		return ErrClosed
	}

	client, err := modbus.NewClient(
		&modbus.ClientConfiguration{
			URL:     "tcp://" + c.config.Address,
			Timeout: c.config.Timeout,
		},
	)
	if err != nil {
		return fmt.Errorf("создание Modbus-клиента: %w", err)
	}

	if err := client.SetUnitId(c.config.UnitID); err != nil {
		return fmt.Errorf("установка Modbus Unit ID: %w", err)
	}

	if err := client.Open(); err != nil {
		return fmt.Errorf(
			"подключение к PLC %s: %w",
			c.config.Address,
			err,
		)
	}

	c.client = client

	return nil
}

func (c *Client) reconnectLocked() error {
	if c.closed {
		return ErrClosed
	}

	if c.client != nil {
		_ = c.client.Close()
		c.client = nil
	}

	if err := c.connectLocked(); err != nil {
		return fmt.Errorf("повторное подключение к PLC: %w", err)
	}

	log.Printf("[plc %s] переподключение...", c.config.Address)

	return nil
}

func (c *Client) readRegisters(
	address uint16,
	quantity uint16,
) ([]uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	if c.client == nil {
		if err := c.connectLocked(); err != nil {
			return nil, err
		}
	}

	registers, firstErr := c.client.ReadRegisters(
		address,
		quantity,
		modbus.HOLDING_REGISTER,
	)
	if firstErr == nil {
		return registers, nil
	}

	log.Printf(
		"[plc %s] ошибка чтения HR%d-HR%d, переподключаюсь: %v",
		c.config.Address, address, address+quantity-1, firstErr,
	)

	if err := c.reconnectLocked(); err != nil {
		return nil, fmt.Errorf(
			"чтение HR%d-HR%d: %w",
			address,
			address+quantity-1,
			errors.Join(firstErr, err),
		)
	}

	registers, retryErr := c.client.ReadRegisters(
		address,
		quantity,
		modbus.HOLDING_REGISTER,
	)
	if retryErr != nil {
		return nil, fmt.Errorf(
			"чтение HR%d-HR%d после reconnect: %w",
			address,
			address+quantity-1,
			errors.Join(firstErr, retryErr),
		)
	}

	return registers, nil
}

func (c *Client) writeRegisters(
	address uint16,
	values []uint16,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClosed
	}

	if len(values) == 0 {
		return errors.New("не переданы регистры для записи")
	}

	if c.client == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}

	if err := c.client.WriteRegisters(address, values); err != nil {
		log.Printf(
			"[plc %s] ошибка записи HR%d, переподключаюсь: %v",
			c.config.Address, address, err,
		)

		reconnectErr := c.reconnectLocked()

		if reconnectErr != nil {
			return fmt.Errorf(
				"запись регистров начиная с HR%d: %w",
				address,
				errors.Join(err, reconnectErr),
			)
		}

		return fmt.Errorf(
			"запись регистров начиная с HR%d: результат операции неизвестен: %w",
			address,
			err,
		)
	}

	return nil
}
