package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gempir/go-twitch-irc/v2"
)

type Runner struct {
	client      *twitch.Client
	channel     string
	bot         string
	timeout     time.Duration
	mu          sync.Mutex
	active      chan twitch.PrivateMessage
	connectedCh chan struct{}
	connectOnce sync.Once
}

func NewRunner(testUser, oauth, channel, bot string, timeout time.Duration) *Runner {
	r := &Runner{
		channel:     channel,
		bot:         strings.ToLower(bot),
		timeout:     timeout,
		connectedCh: make(chan struct{}),
	}
	c := twitch.NewClient(testUser, oauth)
	c.OnConnect(func() {
		r.connectOnce.Do(func() { close(r.connectedCh) })
	})
	c.OnPrivateMessage(func(msg twitch.PrivateMessage) {
		if !strings.EqualFold(msg.User.Name, r.bot) {
			return
		}
		r.mu.Lock()
		ch := r.active
		r.mu.Unlock()
		if ch == nil {
			return
		}
		select {
		case ch <- msg:
		default:
		}
	})
	c.Join(channel)
	r.client = c
	return r
}

func (r *Runner) Connect(timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- r.client.Connect() }()
	select {
	case <-r.connectedCh:
		return nil
	case err := <-errCh:
		if err == nil {
			return errors.New("twitch IRC client exited before connecting")
		}
		return err
	case <-time.After(timeout):
		return errors.New("timed out connecting to twitch IRC")
	}
}

func (r *Runner) Run(cmd Command, params string) Result {
	line := cmd.Trigger
	if params != "" {
		line += " " + params
	}

	ch := make(chan twitch.PrivateMessage, 4)
	r.mu.Lock()
	r.active = ch
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.active = nil
		r.mu.Unlock()
	}()

	res := Result{Trigger: cmd.Trigger, Params: params}
	r.client.Say(r.channel, line)

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	select {
	case msg := <-ch:
		res.Status = "pass"
		res.BotReply = msg.Message
	case <-timer.C:
		if cmd.ExpectsBotReply {
			res.Status = "timeout"
		} else {
			res.Status = "pending-manual"
		}
	}
	return res
}

func (r *Runner) Close() error {
	return r.client.Disconnect()
}
