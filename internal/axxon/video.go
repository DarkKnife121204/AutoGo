package axxon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func (c *Client) VideoFile() string {
	return c.videoFile
}

func (c *Client) Record(ctx context.Context, cameraID string, opened bool) error {
	if cameraID == "" {
		return errors.New("ID видеокамеры не указан")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.photoFile != "" {
		if c.videoFile == "" {
			return errors.New("видеофайл stub не указан")
		}
		return nil
	}
	if c.videoAddress == "" {
		return errors.New("video_address не настроен")
	}
	state := "closed"
	if opened {
		state = "opened"
	}
	body, _ := json.Marshal(map[string]string{"state": state})
	endpoint := c.videoAddress + "/device/di/" + url.PathEscape(cameraID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("формирование команды видео: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("запрос Axxon record: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("ответ Axxon record: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Axxon record вернул HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(bytes.TrimSpace(data)) > 0 {
		var result struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(data, &result) == nil && result.Status != "" && !strings.EqualFold(result.Status, "success") {
			return fmt.Errorf("Axxon record: status=%s", result.Status)
		}
	}
	return nil
}

func validateMP4(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("открытие тестового MP4 %q: %w", path, err)
	}
	defer file.Close()
	header := make([]byte, 8)
	if _, err := io.ReadFull(file, header); err != nil || string(header[4:8]) != "ftyp" {
		return fmt.Errorf("%q не является MP4 (нет ftyp)", path)
	}
	return nil
}
