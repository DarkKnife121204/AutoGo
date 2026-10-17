package plc

const RegisterCount uint16 = 27

const (
	// Входные команды от AutoGo к PLC.

	RegisterInCommand uint16 = 0 // HR0-HR1, Int32, Command
	RegisterInConfig  uint16 = 2 // HR2, Int16
	RegisterInAlarm   uint16 = 3 // HR3, Int16

	// Выходные данные PLC.

	RegisterOutMode        uint16 = 4 // HR4-HR5, Int32, Mode
	RegisterOutState       uint16 = 6 // HR6-HR7, Int32, State
	RegisterOutStateActual uint16 = 8 // HR8-HR9, Int32, ActualState

	RegisterOutAlarm   uint16 = 10 // HR10, Int16, UDTGateAlarm
	RegisterOutWarning uint16 = 11 // HR11, Int16, UDTGateWarning

	RegisterOutLocked        uint16 = 12 // HR12-HR13, Int32
	RegisterOutCommand       uint16 = 14 // HR14-HR15, Int32, Command
	RegisterOutOperationTime uint16 = 16 // HR16-HR17, Int32

	// Входные управляющие данные.

	RegisterInSAUCommand   uint16 = 18 // HR18, Int16
	RegisterInSAUCommandHi uint16 = 19 // HR19, Int16
	RegisterInCommandLock  uint16 = 20 // HR20, Int16
	RegisterInModeLock     uint16 = 21 // HR21, Int16

	// Настройки PLC.

	RegisterTimeWaiting    uint16 = 22 // HR22-HR23, Float32, seconds
	RegisterNumAttempts    uint16 = 24 // HR24, Int16
	RegisterPeriodAttempts uint16 = 25 // HR25-HR26, Float32, seconds
)
