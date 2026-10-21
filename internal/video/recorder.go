package video

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"AutoGo/internal/axxon"
)

var ErrNotRecording = errors.New("видеозапись не ведётся")
var ErrDownloadUnavailable = errors.New("выгрузка MP4 с реального Axxon не поддерживается командным API")

type Recorder struct {
	client   *axxon.Client
	cameraID string
	delay    time.Duration
	duration time.Duration

	mu        sync.Mutex
	active    bool
	wanted    bool
	recording bool
	cancel    context.CancelFunc
	done      chan struct{}
	startTime time.Time
	stopTime  time.Time
}

func New(client *axxon.Client, cameraID string, delay, duration time.Duration) (*Recorder, error) {
	if client == nil || cameraID == "" || delay < 0 || duration <= 0 {
		return nil, errors.New("неверные настройки записи видео")
	}
	return &Recorder{client: client, cameraID: cameraID, delay: delay, duration: duration}, nil
}

func (r *Recorder) StartSequence() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.wanted = true
	if !r.active {
		r.startLocked()
	}
}

func (r *Recorder) startLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	r.active = true
	go r.run(ctx, r.done)
}

func (r *Recorder) StopSequence() {
	r.mu.Lock()
	r.wanted = false
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Recorder) run(ctx context.Context, done chan struct{}) {
	defer func() {
		r.mu.Lock()
		r.active = false
		r.cancel = nil
		restart := ctx.Err() != nil && r.wanted
		if restart {
			r.startLocked()
		}
		r.mu.Unlock()
		close(done)
	}()

	if !wait(ctx, r.delay) {
		return
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := r.client.Record(startCtx, r.cameraID, true)
	cancel()
	if err != nil {
		log.Printf("[video %s] start failed: %v", r.cameraID, err)
		return
	}

	r.mu.Lock()
	r.recording = true
	r.startTime = time.Now()
	r.stopTime = time.Time{}
	r.mu.Unlock()
	log.Printf("[video %s] recording started", r.cameraID)

	wait(ctx, r.duration)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = r.client.Record(stopCtx, r.cameraID, false)
	stopCancel()
	if err != nil {
		log.Printf("[video %s] stop failed: %v", r.cameraID, err)
		return
	}

	r.mu.Lock()
	r.stopTime = time.Now()
	r.recording = false
	r.mu.Unlock()
	log.Printf("[video %s] recording stopped", r.cameraID)
}

func wait(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (r *Recorder) Status() (recording bool, start, stop time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording, r.startTime, r.stopTime
}

func (r *Recorder) CurrentFile() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return "", ErrNotRecording
	}
	if path := r.client.VideoFile(); path != "" {
		return path, nil
	}
	return "", ErrDownloadUnavailable
}

func (r *Recorder) Close() {
	r.StopSequence()
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(21 * time.Second):
		log.Printf("[video %s] shutdown timeout", r.cameraID)
	}
}
