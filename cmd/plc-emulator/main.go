package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"AutoGo/internal/emulator"
	modbusserver "AutoGo/internal/modbus"
)

func main() {
	listenAddress := getEnv(
		"MODBUS_LISTEN_ADDRESS",
		"0.0.0.0:5020",
	)

	openDuration := getDuration(
		"OPEN_DURATION",
		2*time.Second,
	)

	transferDuration := getDuration(
		"TRANSFER_DURATION",
		5*time.Second,
	)

	closeDuration := getDuration(
		"CLOSE_DURATION",
		2*time.Second,
	)

	unitID := getUint8("MODBUS_UNIT_ID", 1)

	log.Printf("PLC Emulator")
	log.Printf("Unit ID: %d", unitID)
	log.Printf("Open duration: %s", openDuration)
	log.Printf("Transfer duration: %s", transferDuration)
	log.Printf("Close duration: %s", closeDuration)

	plcEmulator, err := emulator.New(
		openDuration,
		transferDuration,
		closeDuration,
	)
	if err != nil {
		log.Fatalf("ошибка создания эмулятора: %v", err)
	}

	server := modbusserver.NewServer(
		listenAddress,
		unitID,
		plcEmulator,
	)

	if err := server.Start(); err != nil {
		log.Fatalf("ошибка сервера: %v", err)
	}
}

func getEnv(name string, defaultValue string) string {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	return value
}

func getDuration(name string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf(
			"неверное значение %s=%q: %v",
			name,
			value,
			err,
		)
	}

	return duration
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
