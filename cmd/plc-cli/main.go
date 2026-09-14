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

	"AutoGo/internal/plcclient"
)

const (
	defaultAddress = "127.0.0.1:5020"
	defaultUnitID  = 1
	defaultTimeout = 3 * time.Second
)

func main() {
	address := getEnv(
		"MODBUS_SERVER_ADDRESS",
		defaultAddress,
	)

	unitID := getUint8(
		"MODBUS_UNIT_ID",
		defaultUnitID,
	)

	client, err := plcclient.New(
		plcclient.Config{
			Address: address,
			UnitID:  unitID,
			Timeout: defaultTimeout,
		},
	)
	if err != nil {
		log.Fatalf(
			"не удалось подключиться к PLC: %v",
			err,
		)
	}

	defer func() {
		if err := client.Close(); err != nil {
			log.Printf(
				"ошибка закрытия PLC-соединения: %v",
				err,
			)
		}
	}()

	fmt.Println("AutoGo PLC CLI")
	fmt.Printf(
		"PLC: %s, Unit ID: %d\n",
		address,
		unitID,
	)
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
		log.Printf(
			"ошибка чтения консоли: %v",
			err,
		)
	}
}

func execute(
	client *plcclient.Client,
	line string,
) (bool, error) {
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
		return false, fmt.Errorf(
			"неизвестная команда %q",
			command,
		)
	}
}

func printHelp() {
	fmt.Print(`
Команды:

	status
		Показать текущее состояние PLC.

	registers
		Прочитать все Holding Registers HR0-HR26.

	command <name>
		Отправить команду PLC.

	Доступные команды:
		start
		reverse
		open
		close
		stop
		reset

	watch
		Наблюдать за изменением состояния PLC.
		Для остановки нажмите Ctrl+C.

	exit
		Закрыть приложение.
`)
}

func printStatus(client *plcclient.Client) error {
	status, err := client.Status()
	if err != nil {
		return err
	}

	fmt.Printf(
		"Mode:             %s (%d)\n",
		status.Mode,
		status.Mode,
	)

	fmt.Printf(
		"State:            %s (%d)\n",
		status.State,
		status.State,
	)

	fmt.Printf(
		"Actual state:     %s (%d)\n",
		status.ActualState,
		status.ActualState,
	)

	fmt.Printf(
		"Last command:     %s (%d)\n",
		status.LastCommand,
		status.LastCommand,
	)

	fmt.Printf(
		"Alarm:            %d\n",
		status.Alarm,
	)

	fmt.Printf(
		"Warning:          %d\n",
		status.Warning,
	)

	fmt.Printf(
		"Locked:           %t\n",
		status.Locked,
	)

	fmt.Printf(
		"Operation time:   %.2f s\n",
		status.OperationTime,
	)

	fmt.Printf(
		"Waiting time:     %.2f s\n",
		status.WaitingTime,
	)

	fmt.Printf(
		"Attempts:         %d\n",
		status.Attempts,
	)

	fmt.Printf(
		"Attempt period:   %.2f s\n",
		status.AttemptPeriod,
	)

	return nil
}

func printRegisters(client *plcclient.Client) error {
	registers, err := client.Registers()
	if err != nil {
		return err
	}

	for address, value := range registers {
		fmt.Printf(
			"HR%-2d = %d\n",
			address,
			value,
		)
	}

	return nil
}

func executeCommand(
	client *plcclient.Client,
	args []string,
) error {
	if len(args) != 1 {
		return errors.New(
			"использование: command <name>",
		)
	}

	command := strings.ToLower(args[0])

	var err error

	switch command {
	case "start":
		err = client.Start()

	case "reverse":
		err = client.StartReverse()

	case "open":
		err = client.Open()

	case "close":
		err = client.CloseBarrier()

	case "stop":
		err = client.Stop()

	case "reset":
		err = client.Reset()

	default:
		return fmt.Errorf(
			"неизвестная PLC-команда %q",
			command,
		)
	}

	if err != nil {
		return err
	}

	fmt.Printf(
		"Команда отправлена: %s\n",
		command,
	)

	return nil
}

func watchState(client *plcclient.Client) error {
	previousState := ""

	fmt.Println(
		"Наблюдение запущено. Для остановки нажмите Ctrl+C.",
	)

	for {
		status, err := client.Status()
		if err != nil {
			fmt.Printf(
				"Ошибка чтения: %v\n",
				err,
			)

			time.Sleep(time.Second)
			continue
		}

		currentState := status.State.String()

		if currentState != previousState {
			fmt.Printf(
				"%s State: %s (%d)\n",
				time.Now().Format("15:04:05"),
				status.State,
				status.State,
			)

			previousState = currentState
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func getEnv(
	name string,
	defaultValue string,
) string {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	return value
}

func getUint8(
	name string,
	defaultValue uint8,
) uint8 {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	number, err := strconv.ParseUint(
		value,
		10,
		8,
	)
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
