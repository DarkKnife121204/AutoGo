package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf(
			"чтение конфигурации %q: %w",
			path,
			err,
		)
	}

	var config Config

	expanded := os.ExpandEnv(string(data))

	decoder := yaml.NewDecoder(
		strings.NewReader(expanded),
	)

	decoder.KnownFields(true)

	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf(
			"разбор YAML-конфигурации %q: %w",
			path,
			err,
		)
	}

	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf(
			"проверка YAML-конфигурации %q: %w",
			path,
			err,
		)
	}

	return config, nil
}
