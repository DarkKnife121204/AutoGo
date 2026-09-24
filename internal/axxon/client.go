package axxon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Address  string
	Server   string
	Username string
	Password string
	Timeout  time.Duration
}

type Client struct {
	address  string
	server   string
	username string
	password string

	httpClient *http.Client
}

func New(config Config) (*Client, error) {
	address := strings.TrimRight(
		strings.TrimSpace(config.Address),
		"/",
	)

	if address == "" {
		return nil, errors.New(
			"адрес Axxon не указан",
		)
	}

	parsedURL, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf(
			"разбор адреса Axxon %q: %w",
			address,
			err,
		)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf(
			"некорректный адрес Axxon %q",
			address,
		)
	}

	server := strings.TrimSpace(config.Server)
	if server == "" {
		return nil, errors.New(
			"имя сервера Axxon не указано",
		)
	}

	if config.Timeout <= 0 {
		return nil, errors.New(
			"таймаут Axxon должен быть больше нуля",
		)
	}

	return &Client{
		address:  address,
		server:   server,
		username: config.Username,
		password: config.Password,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

func (c *Client) Snapshot(
	ctx context.Context,
	cameraID string,
) ([]byte, error) {
	cameraID = strings.TrimSpace(cameraID)

	if cameraID == "" {
		return nil, errors.New(
			"ID камеры Axxon не указан",
		)
	}

	snapshotURL :=
		c.address +
			"/live/media/snapshot/" +
			url.PathEscape(c.server) +
			"/DeviceIpint." +
			url.PathEscape(cameraID) +
			"/SourceEndpoint.video:0:0"

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		snapshotURL,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"создание запроса snapshot Axxon: %w",
			err,
		)
	}

	request.Header.Set(
		"Accept",
		"image/jpeg",
	)

	if c.username != "" || c.password != "" {
		request.SetBasicAuth(
			c.username,
			c.password,
		)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf(
			"получение snapshot Axxon: %w",
			err,
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(
			io.LimitReader(
				response.Body,
				4096,
			),
		)

		return nil, fmt.Errorf(
			"Axxon snapshot вернул HTTP %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	image, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf(
			"чтение JPEG Axxon: %w",
			err,
		)
	}

	if len(image) == 0 {
		return nil, errors.New(
			"Axxon вернул пустой snapshot",
		)
	}

	return image, nil
}
