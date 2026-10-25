package onbon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
)

const queueSize = 64

type command struct {
	areaID int
	text   string
	color  string
	clear  bool
}

type Board struct {
	ID     string
	client *Client
	queue  chan command

	ctx      context.Context
	cancel   context.CancelFunc
	finished chan struct{}
	once     sync.Once
}

func NewBoard(id string, client *Client) (*Board, error) {
	if id == "" || client == nil {
		return nil, errors.New("Onbon: ID или клиент не указан")
	}

	ctx, cancel := context.WithCancel(context.Background())

	board := &Board{
		ID:       id,
		client:   client,
		queue:    make(chan command, queueSize),
		ctx:      ctx,
		cancel:   cancel,
		finished: make(chan struct{}),
	}

	go board.run()

	return board, nil
}

func (b *Board) run() {
	defer close(b.finished)

	for {
		select {
		case <-b.ctx.Done():
			return

		case cmd := <-b.queue:
			timeout := 9 * b.client.http.Timeout

			ctx, cancel := context.WithTimeout(b.ctx, timeout)

			var err error

			if cmd.clear {
				err = b.client.ClearAll(ctx)
			} else {
				err = b.client.SendDynamic(
					ctx,
					cmd.areaID,
					cmd.text,
					cmd.color,
				)
			}

			cancel()

			if err != nil {
				log.Printf(
					"[onbon %s] ошибка команды: %v",
					b.ID, err,
				)
			} else if cmd.clear {
				log.Printf("[onbon %s] CLEAR OK", b.ID)
			} else {
				log.Printf(
					"[onbon %s] SEND OK: area=%d text=%q color=%s",
					b.ID,
					cmd.areaID,
					cmd.text,
					cmd.color,
				)
			}
		}
	}
}

func (b *Board) enqueue(cmd command) bool {
	if b == nil {
		return false
	}

	select {
	case <-b.ctx.Done():
		return false
	default:
	}

	select {
	case <-b.ctx.Done():
		return false

	case b.queue <- cmd:
		return true

	default:
		log.Printf("[onbon %s] очередь заполнена", b.ID)
		return false
	}
}

func (b *Board) Send(
	areaID int,
	text string,
	color string,
) bool {
	return b.enqueue(command{
		areaID: areaID,
		text:   text,
		color:  color,
	})
}

func (b *Board) Clear() bool {
	if b == nil {
		return false
	}

drain:
	for {
		select {
		case <-b.queue:
		default:
			break drain
		}
	}

	return b.enqueue(command{clear: true})
}

func (b *Board) Close() {
	if b == nil {
		return
	}

	b.once.Do(func() {
		b.cancel()
	})

	<-b.finished
}

type Display struct {
	board     *Board
	plateArea int
	infoArea  int
}

func NewDisplay(
	board *Board,
	plateArea int,
	infoArea int,
) (*Display, error) {
	if board == nil {
		return nil, errors.New("контроллер Onbon отсутствует")
	}

	if plateArea == infoArea {
		return nil, errors.New("области номера и сообщений должны отличаться")
	}

	if !board.client.HasArea(plateArea) ||
		!board.client.HasArea(infoArea) {
		return nil, fmt.Errorf(
			"области Onbon %d или %d не найдены",
			plateArea, infoArea,
		)
	}

	return &Display{
		board:     board,
		plateArea: plateArea,
		infoArea:  infoArea,
	}, nil
}

func (d *Display) ShowPlate(plate, plateType string) bool {
	if d == nil {
		return false
	}

	return d.board.Send(
		d.plateArea,
		plate,
		PlateColor(plateType),
	)
}

func (d *Display) ShowMessage(message MessageType) bool {
	if d == nil {
		return false
	}

	text, color, ok := MessageInfo(message)
	if !ok {
		return false
	}

	return d.board.Send(d.infoArea, text, color)
}

func (d *Display) Clear() bool {
	if d == nil {
		return false
	}

	return d.board.Clear()
}
