package access

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LaravelDecider struct {
	baseURL   string
	client    *http.Client
	gateNames map[string]string
}

func NewLaravelDecider(address string, timeout time.Duration, gateNames map[string]string) (*LaravelDecider, error) {
	address = strings.TrimRight(strings.TrimSpace(address), "/")

	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("некорректный адрес API Laravel: %q", address)
	}

	if timeout <= 0 {
		return nil, errors.New("таймаут Laravel должен быть больше нуля")
	}

	return &LaravelDecider{
		baseURL:   address,
		client:    &http.Client{Timeout: timeout},
		gateNames: gateNames,
	}, nil
}

func (d *LaravelDecider) Decide(ctx context.Context, request Request) (Decision, error) {
	gateName := strings.TrimSpace(d.gateNames[request.LaneID])
	if gateName == "" {
		return Decision{}, errors.New("gate_name не указан для проверки Laravel")
	}

	var direction string

	switch request.Direction {
	case "normal":
		direction = "Forward"
	case "reverse":
		direction = "Backward"
	default:
		return Decision{}, fmt.Errorf("неизвестное направление: %q", request.Direction)
	}

	payload := map[string]string{
		"gate_name": gateName,
		"direction": direction,
	}

	var path string

	switch {
	case request.Plate != "" && request.KeyCode == "":
		path = "/checkpoint/plate"
		payload["plate"] = request.Plate

	case request.KeyCode != "" && request.Plate == "":
		path = "/checkpoint/sms"
		payload["code"] = request.KeyCode

	default:
		return Decision{}, errors.New("нужно указать только госномер или SMS-код")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, fmt.Errorf("кодирование запроса Laravel: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		d.baseURL+path,
		bytes.NewReader(body),
	)
	if err != nil {
		return Decision{}, fmt.Errorf("создание запроса Laravel: %w", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	response, err := d.client.Do(httpRequest)
	if err != nil {
		return Decision{}, fmt.Errorf("проверка через Laravel: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return Decision{}, fmt.Errorf("Laravel %s вернул HTTP %d", path, response.StatusCode)
	}

	var data struct {
		Allowed   *bool  `json:"allowed"`
		PlateType string `json:"plate_type"`
	}

	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&data); err != nil {
		return Decision{}, fmt.Errorf("некорректный ответ Laravel: %w", err)
	}

	if data.Allowed == nil {
		return Decision{}, errors.New("Laravel не вернул allowed")
	}

	plateType := PlateType(data.PlateType)

	switch plateType {
	case PlateTypeRegular, PlateTypeVIP, PlateTypeInvalid, PlateTypeUnknown:
	default:
		return Decision{}, fmt.Errorf("неизвестный plate_type от Laravel: %q", data.PlateType)
	}

	allowed := *data.Allowed &&
		(plateType == PlateTypeRegular || plateType == PlateTypeVIP)

	return Decision{
		Allowed:   allowed,
		PlateType: plateType,
	}, nil
}
