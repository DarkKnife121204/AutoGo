package onbon

import "strings"

type MessageType string

const (
	MessageStop      MessageType = "stop"
	MessageWait      MessageType = "wait"
	MessageGranted   MessageType = "granted"
	MessageDenied    MessageType = "denied"
	MessageGiveWay   MessageType = "give_way"
	MessageForbidden MessageType = "forbidden"
	MessageBadPlate  MessageType = "bad_plate"
	MessageBusy      MessageType = "busy"
	MessageError     MessageType = "error"
	MessageNotReady  MessageType = "not_ready"
)

const (
	ColorWhite  = "0xFFFFFFFF"
	ColorGreen  = "0xFF00FF00"
	ColorRed    = "0xFFFF0000"
	ColorYellow = "0xFFFFFF00"
	ColorOrange = "0xFFFFA500"
)

type messageInfo struct {
	text  string
	color string
}

var messages = map[MessageType]messageInfo{
	MessageStop: {
		"ОСТАНОВИТЕСЬ\nУ СТОП ЛИНИИ",
		ColorYellow,
	},
	MessageWait: {
		"ОЖИДАЙТЕ\nОТКРЫТИЯ\nШЛАГБАУМА",
		ColorWhite,
	},
	MessageGranted: {
		"ПРОЕЗД\nРАЗРЕШЕН",
		ColorGreen,
	},
	MessageDenied: {
		"ПРОПУСК\nНЕ НАЙДЕН",
		ColorRed,
	},
	MessageGiveWay: {
		"УСТУПИТЕ\nДОРОГУ",
		ColorYellow,
	},
	MessageForbidden: {
		"ПРОЕЗД\nЗАКРЫТ",
		ColorRed,
	},
	MessageBadPlate: {
		"ОШИБКА\nСЧИТЫВАНИЯ\nНОМЕРА",
		ColorRed,
	},
	MessageBusy: {
		"ПРОЕЗД ЗАНЯТ\nОЖИДАЙТЕ\nСВОЕЙ ОЧЕРЕДИ",
		ColorOrange,
	},
	MessageError: {
		"ОШИБКА\nАВТОМАТИКИ",
		ColorRed,
	},
	MessageNotReady: {
		"НЕТ ГОТОВНОСТИ\nАВТОМАТИКИ",
		ColorRed,
	},
}

func MessageInfo(message MessageType) (string, string, bool) {
	info, exists := messages[message]
	if !exists {
		return "", "", false
	}

	return info.text, info.color, true
}

func PlateColor(plateType string) string {
	switch strings.ToLower(plateType) {
	case "regular", "vip":
		return ColorGreen

	case "invalid":
		return ColorRed

	default:
		return ColorWhite
	}
}
