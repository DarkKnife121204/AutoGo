package lanes

import (
	"AutoGo/internal/scenarios"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type VehicleContext struct {
	ID        string
	Value     string
	Source    string
	Direction string
	Waiting   bool
	StartedAt time.Time

	source scenarios.TriggerSource
}

func newVehicleID() string {
	buf := make([]byte, 16)

	if _, err := rand.Read(buf); err != nil {
		return "veh-" + time.Now().Format("20060102150405.000000000")
	}

	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	return hex.EncodeToString(buf[0:4]) + "-" +
		hex.EncodeToString(buf[4:6]) + "-" +
		hex.EncodeToString(buf[6:8]) + "-" +
		hex.EncodeToString(buf[8:10]) + "-" +
		hex.EncodeToString(buf[10:16])
}
