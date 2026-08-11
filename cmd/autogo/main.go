package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"AutoGo/internal/config"
	"AutoGo/internal/httpapi"
	"AutoGo/internal/runtime"
)

const (
	defaultHTTPAddress = ":5001"
)

func main() {
	configPath := getEnv(
		"AUTOGO_CONFIG_PATH",
		"config/site.yaml",
	)

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf(
			"не удалось загрузить конфигурацию AutoGo: %v",
			err,
		)
	}

	log.Printf(
		"конфигурация загружена: site=%s controllers=%d devices=%d checkpoints=%d",
		cfg.Site.ID,
		len(cfg.Controllers),
		len(cfg.Devices),
		len(cfg.Checkpoints),
	)

	httpAddress := getEnv(
		"AUTOGO_HTTP_ADDRESS",
		defaultHTTPAddress,
	)

	rt, err := runtime.Build(cfg)
	if err != nil {
		log.Fatalf(
			"не удалось собрать runtime AutoGo: %v",
			err,
		)
	}

	defer rt.Close()

	rt.StartPollers()

	defer rt.StopPollers()

	log.Printf(
		"runtime собран: controllers=%d barriers=%d lanes=%d checkpoints=%d",
		len(rt.Controllers),
		len(rt.Barriers),
		len(rt.Lanes),
		len(rt.Checkpoints),
	)

	api := httpapi.New(
		rt.Barriers,
		rt.Lanes,
		rt.BarrierLane,
		rt.TriggerIndex,
		rt.Checkpoints,
	)

	server := &http.Server{
		Addr:              httpAddress,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf(
			"AutoGo HTTP Server запущен: %s",
			httpAddress,
		)

		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)

	signal.Notify(
		shutdownSignal,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf(
				"ошибка HTTP-сервера: %v",
				err,
			)
		}

	case sig := <-shutdownSignal:
		log.Printf(
			"получен сигнал завершения: %s",
			sig,
		)

		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf(
				"ошибка graceful shutdown: %v",
				err,
			)

			_ = server.Close()
		}
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
