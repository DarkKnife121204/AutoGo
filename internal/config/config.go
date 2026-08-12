package config

import "time"

type Config struct {
	Version     int                `yaml:"version"`
	Site        SiteConfig         `yaml:"site"`
	Controllers []ControllerConfig `yaml:"controllers"`
	Devices     []DeviceConfig     `yaml:"devices"`
	Checkpoints []CheckpointConfig `yaml:"checkpoints"`
}

type SiteConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type ControllerConfig struct {
	ID      string        `yaml:"id"`
	Type    string        `yaml:"type"`
	Address string        `yaml:"address"`
	UnitID  uint8         `yaml:"unit_id"`
	Timeout time.Duration `yaml:"timeout"`
}

type DeviceConfig struct {
	ID         string `yaml:"id"`
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Controller string `yaml:"controller"`
	ExternalID string `yaml:"external_id"`
	Direction  string `yaml:"direction"`
}

type CheckpointConfig struct {
	ID    string       `yaml:"id"`
	Name  string       `yaml:"name"`
	Lanes []LaneConfig `yaml:"lanes"`
}

type LaneConfig struct {
	ID       string         `yaml:"id"`
	Name     string         `yaml:"name"`
	Mode     string         `yaml:"mode"`
	Devices  []string       `yaml:"devices"`
	Scenario ScenarioConfig `yaml:"scenario"`
}

type ScenarioConfig struct {
	Type     string           `yaml:"type"`
	Settings ScenarioSettings `yaml:"settings"`
}

type ScenarioSettings struct {
	Barrier string `yaml:"barrier"`

	EntryBarrier string `yaml:"entry_barrier"`
	ExitBarrier  string `yaml:"exit_barrier"`

	ReleaseMode string `yaml:"release_mode"`
	QueueDepth  int    `yaml:"queue_depth"`
}
