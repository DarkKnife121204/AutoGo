package access

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ContextUpdate struct {
	LaneID         string
	Direction      string
	State          string
	Identification string
}

func (d *LaravelDecider) NotifyContext(ctx context.Context, update ContextUpdate) error {
	gateName := strings.TrimSpace(d.gateNames[update.LaneID])
	if gateName == "" {
		return fmt.Errorf("не найден gate_name для линии %q", update.LaneID)
	}

	var direction string
	switch update.Direction {
	case "normal":
		direction = "Forward"
	case "reverse":
		direction = "Backward"
	default:
		return fmt.Errorf("неизвестное направление: %q", update.Direction)
	}

	switch update.State {
	case "NotReady", "IdentEntrance", "WaitingTransfer1", "Confirm",
		"WaitingTransfer2", "Alarm", "Stopping", "RollingBack":
	default:
		return fmt.Errorf("Laravel не поддерживает состояние %q", update.State)
	}

	body, err := json.Marshal(struct {
		GateName       string  `json:"gate_name"`
		Direction      string  `json:"direction"`
		State          string  `json:"state"`
		Identification *string `json:"identifikator"`
	}{
		GateName:       gateName,
		Direction:      direction,
		State:          update.State,
		Identification: optionalIdentification(update.Identification),
	})
	if err != nil {
		return fmt.Errorf("кодирование контекста Laravel: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		d.baseURL+"/checkpoint/context",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("создание запроса контекста Laravel: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("отправка контекста Laravel: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Laravel /checkpoint/context вернул HTTP %d", resp.StatusCode)
	}

	var result struct {
		Allowed *bool `json:"allowed"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&result); err != nil {
		return fmt.Errorf("некорректный ответ Laravel: %w", err)
	}

	if result.Allowed == nil || !*result.Allowed {
		return errors.New("Laravel не подтвердил обновление контекста")
	}

	return nil
}

func optionalIdentification(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
