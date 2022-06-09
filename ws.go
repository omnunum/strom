package main

import (
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
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
	URL     url.URL
	Ping    Ping
	Receive chan []byte
	Init    func(send chan []byte)
}

func (s Subscription) run() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	log.Printf("connecting to %s", s.URL.String())

	c, _, err := websocket.DefaultDialer.Dial(s.URL.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan bool)

	var last_received_at time.Time

	go func() {
		defer func() { done <- true }()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				break
			}
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
				log.Println("write:", err)
				return
			}
		}
	}()

	// This is usually where we send the subscription message
	s.Init(send)

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case recv := <-s.Receive:
			log.Printf("recv: %s", recv)
			last_received_at = time.Now()
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
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)
			if err != nil {
				log.Println("write close:", err)
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
