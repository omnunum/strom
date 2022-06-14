package main

import (
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type Frame struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

type Ping struct {
	Interval time.Duration
	Message  func() string
}

type Subscription struct {
	Request http.Request
	Ping    Ping
	Receive chan []byte
	Init    func(send chan []byte)
	Wait    *sync.WaitGroup
}

func (s Subscription) run() {
	defer s.Wait.Done()

	flag.Parse()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	url := s.Request.URL.String()
	c, _, err := websocket.DefaultDialer.Dial(url, s.Request.Header)
	if err != nil {
		log.Fatal().Err(err).Msg("Dialing")
		return
	}
	log.Info().Str("url", url).Msg("Dialed")
	defer c.Close()

	done := make(chan bool)

	var last_received_at time.Time

	go func() {
		defer func() { done <- true }()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Error().Err(err).Msg("Receiving")
				break
			}
			last_received_at = time.Now()
			s.Receive <- message
		}
	}()

	send := make(chan []byte)
	defer close(send)

	go func() {
		defer func() { done <- true }()
		for {
			msg := <-send
			err := c.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Error().RawJSON("msg", msg).Err(err).Msg("Sending")
				return
			}
			log.Info().RawJSON("msg", msg).Msg("Sent")

		}
	}()

	// This is usually where we send the subscription message
	s.Init(send)

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			close(done)
			return
		case <-ticker.C:
			// Every second, check if we've recieved a message
			// in the past 6 seconds, and send a ping if we haven't
			if time.Since(last_received_at) >= s.Ping.Interval {
				send <- []byte(s.Ping.Message())
			}
		case <-interrupt:
			log.Info().Msg("Interrupted")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)
			if err != nil {
				log.Error().Err(err).Msg("Closing writer")
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
