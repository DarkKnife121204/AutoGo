package plc

const RegisterCount uint16 = 27

const (
	RegisterInCommand uint16 = 0 // HR0-HR1, Int32
	RegisterInConfig  uint16 = 2
	RegisterInAlarm   uint16 = 3

	RegisterOutMode        uint16 = 4 // HR4-HR5, Int32
	RegisterOutState       uint16 = 6 // HR6-HR7, Int32
	RegisterOutStateActual uint16 = 8 // HR8-HR9, Int32

	RegisterOutAlarm   uint16 = 10
	RegisterOutWarning uint16 = 11

	RegisterOutLocked        uint16 = 12 // HR12-HR13, Int32
	RegisterOutCommand       uint16 = 14 // HR14-HR15, Int32
	RegisterOutOperationTime uint16 = 16 // HR16-HR17, Float32

	RegisterInSAUCommand   uint16 = 18
	RegisterInSAUCommandHi uint16 = 19
	RegisterInCommandLock  uint16 = 20
	RegisterInModeLock     uint16 = 21

	RegisterTimeWaiting    uint16 = 22 // HR22-HR23, Float32
	RegisterNumAttempts    uint16 = 24
	RegisterPeriodAttempts uint16 = 25 // HR25-HR26, Float32
)
