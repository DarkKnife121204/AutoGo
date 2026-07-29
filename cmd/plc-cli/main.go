package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	modbus "github.com/simonvetter/modbus"

	"AutoGo/internal/plc"
)

const (
	defaultAddress = "127.0.0.1:5020"
	defaultUnitID  = 1
)

type PLCClient struct {
	address string
	unitID  uint8
	timeout time.Duration
	client  *modbus.ModbusClient
}

func main() {
	client := &PLCClient{
		address: getEnv("MODBUS_SERVER_ADDRESS", defaultAddress),
		unitID:  getUint8("MODBUS_UNIT_ID", defaultUnitID),
		timeout: 3 * time.Second,
	}

	if err := client.connect(); err != nil {
		log.Fatalf("не удалось подключиться к PLC: %v", err)
	}
	defer client.Close()

	fmt.Println("AutoGo PLC CLI")
	fmt.Printf("PLC: %s, Unit ID: %d\n", client.address, client.unitID)
	fmt.Println(`Введите "help" для списка команд.`)

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("plc> ")

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		shouldExit, err := execute(client, line)
		if err != nil {
			fmt.Printf("Ошибка: %v\n", err)
		}

		if shouldExit {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ошибка чтения консоли: %v", err)
	}
}

func execute(client *PLCClient, line string) (bool, error) {
	parts := strings.Fields(line)
	command := strings.ToLower(parts[0])
	args := parts[1:]

	switch command {
	case "help":
		printHelp()
		return false, nil

	case "status":
		return false, printStatus(client)

	case "registers":
		return false, printRegisters(client)

	case "command":
		return false, executeCommand(client, args)

	case "watch":
		return false, watchState(client)

	case "exit", "quit":
		return true, nil

	default:
		return false, fmt.Errorf("неизвестная команда %q", command)
	}
}

func printHelp() {
	fmt.Println(`
Команды:

  	status
		Показать состояние, последнюю команду и аварию.

  	registers
 		Прочитать все HR0-HR26.

  	command <name>
		Отправить команду в HR0-HR1.

	Доступные команды:
		start
		reverse
		open
		close
		stop
		reset
		none

	watch
		Постоянно выводить состояние PLC.
		Для остановки нажмите Ctrl+C.

  	exit
		Закрыть приложение.
`)
}

func printStatus(client *PLCClient) error {
	registers, err := client.readRegisters(0, plc.RegisterCount)
	if err != nil {
		return err
	}

	state := plc.State(
		plc.DecodeInt32(
			registers[plc.RegisterOutState],
			registers[plc.RegisterOutState+1],
		),
	)

	actualState := plc.State(
		plc.DecodeInt32(
			registers[plc.RegisterOutStateActual],
			registers[plc.RegisterOutStateActual+1],
		),
	)

	lastCommand := plc.Command(
		plc.DecodeInt32(
			registers[plc.RegisterOutCommand],
			registers[plc.RegisterOutCommand+1],
		),
	)

	mode := plc.Mode(
		plc.DecodeInt32(
			registers[plc.RegisterOutMode],
			registers[plc.RegisterOutMode+1],
		),
	)

	locked := plc.DecodeInt32(
		registers[plc.RegisterOutLocked],
		registers[plc.RegisterOutLocked+1],
	) != 0

	operationTime := plc.DecodeFloat32(
		registers[plc.RegisterOutOperationTime],
		registers[plc.RegisterOutOperationTime+1],
	)

	waitingTime := plc.DecodeFloat32(
		registers[plc.RegisterTimeWaiting],
		registers[plc.RegisterTimeWaiting+1],
	)

	attempts := registers[plc.RegisterNumAttempts]

	attemptPeriod := plc.DecodeFloat32(
		registers[plc.RegisterPeriodAttempts],
		registers[plc.RegisterPeriodAttempts+1],
	)

	alarm := plc.Alarm(registers[plc.RegisterOutAlarm])

	fmt.Printf("Mode:             %s (%d)\n", mode, mode)
	fmt.Printf("State:            %s (%d)\n", state, state)
	fmt.Printf("Actual state:     %s (%d)\n", actualState, actualState)
	fmt.Printf("Last command:     %s (%d)\n", lastCommand, lastCommand)
	fmt.Printf("Alarm:            %d\n", alarm)
	fmt.Printf("Locked:           %t\n", locked)
	fmt.Printf("Operation time:   %.2f s\n", operationTime)
	fmt.Printf("Waiting time:     %.2f s\n", waitingTime)
	fmt.Printf("Attempts:         %d\n", attempts)
	fmt.Printf("Attempt period:   %.2f s\n", attemptPeriod)

	return nil
}

func printRegisters(client *PLCClient) error {
	registers, err := client.readRegisters(0, plc.RegisterCount)
	if err != nil {
		return err
	}

	for address, value := range registers {
		fmt.Printf("HR%-2d = %d\n", address, value)
	}

	return nil
}

func executeCommand(client *PLCClient, args []string) error {
	if len(args) != 1 {
		return errors.New("использование: command <name>")
	}

	command, err := parseCommand(args[0])
	if err != nil {
		return err
	}

	registers := plc.EncodeInt32(int32(command))

	if err := client.writeRegisters(
		plc.RegisterInCommand,
		registers[:],
	); err != nil {
		return err
	}

	fmt.Printf("Команда отправлена: %s (%d)\n", command, command)

	return nil
}

func watchState(client *PLCClient) error {
	var previousState plc.State = -1

	fmt.Println("Наблюдение запущено. Для остановки нажмите Ctrl+C.")

	for {
		registers, err := client.readRegisters(
			plc.RegisterOutState,
			2,
		)
		if err != nil {
			fmt.Printf("Ошибка чтения: %v\n", err)
			time.Sleep(time.Second)
			continue
		}

		state := plc.State(
			plc.DecodeInt32(registers[0], registers[1]),
		)

		if state != previousState {
			fmt.Printf(
				"%s State: %s (%d)\n",
				time.Now().Format("15:04:05"),
				state,
				state,
			)

			previousState = state
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func (c *PLCClient) readRegisters(
	address uint16,
	quantity uint16,
) ([]uint16, error) {
	if c.client == nil {
		return nil, errors.New(
			"нет подключения к PLC",
		)
	}

	return c.client.ReadRegisters(
		address,
		quantity,
		modbus.HOLDING_REGISTER,
	)
}

func (c *PLCClient) writeRegisters(
	address uint16,
	values []uint16,
) error {
	if c.client == nil {
		return errors.New(
			"нет подключения к PLC",
		)
	}

	return c.client.WriteRegisters(
		address,
		values,
	)
}

func (c *PLCClient) Close() {
	if c.client == nil {
		return
	}

	if err := c.client.Close(); err != nil {
		log.Printf(
			"ошибка закрытия PLC-соединения: %v",
			err,
		)
	}
}

func (c *PLCClient) connect() error {
	client, err := modbus.NewClient(
		&modbus.ClientConfiguration{
			URL:     "tcp://" + c.address,
			Timeout: c.timeout,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"создание Modbus-клиента: %w",
			err,
		)
	}

	if err := client.SetUnitId(c.unitID); err != nil {
		return fmt.Errorf(
			"установка Unit ID: %w",
			err,
		)
	}

	if err := client.Open(); err != nil {
		return fmt.Errorf(
			"подключение к %s: %w",
			c.address,
			err,
		)
	}

	c.client = client

	return nil
}

func parseCommand(value string) (plc.Command, error) {
	switch strings.ToLower(value) {
	case "none":
		return plc.CommandNone, nil
	case "stop":
		return plc.CommandStop, nil
	case "start":
		return plc.CommandStart, nil
	case "reverse":
		return plc.CommandStartReverse, nil
	case "open":
		return plc.CommandOpen, nil
	case "close":
		return plc.CommandClose, nil
	case "reset":
		return plc.CommandReset, nil
	default:
		return plc.CommandNone, fmt.Errorf(
			"неизвестная PLC-команда %q",
			value,
		)
	}
}

func getEnv(name string, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}

	return value
}

func getUint8(name string, defaultValue uint8) uint8 {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}

	number, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		log.Fatalf(
			"неверное значение %s=%q: %v",
			name,
			value,
			err,
		)
	}

	return uint8(number)
}
