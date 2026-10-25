package onbon

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"AutoGo/internal/config"
)

type Client struct {
	address  string
	username string
	password string
	session  string

	areas map[int]config.OnbonAreaConfig
	http  *http.Client
	mu    sync.Mutex
}

func NewClient(cfg config.ServiceConfig) (*Client, error) {
	address := strings.TrimRight(cfg.Address, "/")

	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "http" ||
		parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, fmt.Errorf("некорректный адрес Onbon: %q", address)
	}

	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("не указан timeout Onbon")
	}

	areas := make(map[int]config.OnbonAreaConfig)

	for _, area := range cfg.Areas {
		if _, exists := areas[area.ID]; exists {
			return nil, fmt.Errorf("дублируется область Onbon %d", area.ID)
		}
		areas[area.ID] = area
	}

	return &Client{
		address:  address,
		username: cfg.Username,
		password: cfg.Password,
		areas:    areas,
		http:     &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (c *Client) HasArea(id int) bool {
	_, exists := c.areas[id]
	return exists
}

func sha1Hash(value string) string {
	hash := sha1.Sum([]byte(value))
	return hex.EncodeToString(hash[:])
}

func requestBody(name string, input any) any {
	return map[string]any{
		"protocol": map[string]any{
			"name":    "YQ-COM2",
			"version": "1.0",
			"remotefunction": map[string]any{
				"name":  name,
				"input": input,
			},
		},
	}
}

func (c *Client) post(
	ctx context.Context,
	address string,
	body any,
) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		address,
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"Onbon HTTP %d: %s",
			resp.StatusCode,
			string(result),
		)
	}

	if !json.Valid(result) {
		return nil, fmt.Errorf("Onbon вернул некорректный JSON")
	}

	if bytes.Contains(bytes.ToLower(result), []byte("error")) {
		return nil, fmt.Errorf("Onbon: %s", string(result))
	}

	return result, nil
}

func outputField(data []byte, field string) (string, error) {
	var root map[string]any

	if err := json.Unmarshal(data, &root); err != nil {
		return "", err
	}

	if protocol, ok := root["protocol"].(map[string]any); ok {
		root = protocol
	}

	remote, ok := root["remotefunction"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("нет remotefunction")
	}

	output, ok := remote["output"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("нет output")
	}

	value, ok := output[field].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("нет поля %s", field)
	}

	return value, nil
}

func (c *Client) login(ctx context.Context) (string, error) {
	if c.session != "" {
		return c.session, nil
	}

	response, err := c.post(
		ctx,
		c.address+"/",
		requestBody("GetVerificationCode", map[string]any{
			"username": c.username,
		}),
	)
	if err != nil {
		return "", err
	}

	code, err := outputField(response, "verificationcode")
	if err != nil {
		return "", err
	}

	password := sha1Hash(code + sha1Hash(c.password))

	response, err = c.post(
		ctx,
		c.address+"/",
		requestBody("UserLogin", map[string]any{
			"username":         c.username,
			"verificationcode": code,
			"password":         password,
		}),
	)
	if err != nil {
		return "", err
	}

	session, err := outputField(response, "sessionID")
	if err != nil {
		return "", err
	}

	c.session = session
	return session, nil
}

func (c *Client) execute(
	ctx context.Context,
	body any,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 1 {
			c.session = ""
		}

		session, err := c.login(ctx)

		if err == nil {
			address := c.address + "/;stok=" +
				url.PathEscape(session) + "/"

			_, err = c.post(ctx, address, body)
		}

		if err == nil {
			return nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf(
		"Onbon: ошибка после 3 попыток: %w",
		lastErr,
	)
}

func (c *Client) SendDynamic(
	ctx context.Context,
	areaID int,
	text string,
	color string,
) error {
	area, exists := c.areas[areaID]
	if !exists {
		return fmt.Errorf("область Onbon %d не найдена", areaID)
	}

	if color == "" {
		color = "0xFFFFFFFF"
	}

	number := strconv.Itoa

	body := requestBody("UpdateDynamic", map[string]any{
		"immediatelyPlay": number(areaID),
		"cover":           "0",
		"dynamics": []any{
			map[string]any{
				"id":           number(areaID),
				"xCoord":       number(area.X),
				"yCoord":       number(area.Y),
				"width":        number(area.Width),
				"height":       number(area.Height),
				"transparency": "100",
				"unit": []any{
					map[string]any{
						"type":       "Text",
						"command":    "0",
						"order":      "0",
						"stuntType":  "2",
						"stuntSpeed": "1",
						"content": base64.StdEncoding.EncodeToString(
							[]byte(text),
						),
						"fontSize":    number(area.FontSize),
						"fontColor":   color,
						"alignment_H": "2",
						"alignment_V": "2",
					},
				},
			},
		},
	})

	return c.execute(ctx, body)
}

func (c *Client) ClearAll(ctx context.Context) error {
	return c.execute(
		ctx,
		requestBody("clearDynamic", map[string]any{
			"id": []string{"*"},
		}),
	)
}
