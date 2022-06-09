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

var addr = flag.String("addr", "ws-draftkingseu.pusher.com", "http service address")

type Frame struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

var ping = "{\"event\": \"pusher:ping\", \"data\": \"{}\"}"

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{
		Scheme: "ws", Host: *addr, Path: "app/490c3809b82ef97880f2",
		RawQuery: "protocol=7&client=js&version=4.2.2&flash=false",
	}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan bool)

	var last_received_at time.Time
	receive := make(chan []byte)
	defer close(receive)

	go func() {
		defer func() { done <- true }()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				break
			}
			receive <- message
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

	send <- []byte("{\"event\":\"pusher:subscribe\",\"data\":{\"channel\":\"nj_ent-eventgroup-88670847\"}}")

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case recv := <-receive:
			log.Printf("recv: %s", recv)
			last_received_at = time.Now()
		case <-done:
			close(done)
			return
		case <-ticker.C:
			// Every second, check if we've recieved a message
			// in the past 6 seconds, and send a ping if we haven't
			if time.Since(last_received_at).Seconds() >= 6 {
				send <- []byte(ping)
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
