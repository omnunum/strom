package main

import (
	"net/url"
	"time"
)

func main() {
	recv := make(chan []byte)
	defer close(recv)

	sub := Subscription{
		URL: url.URL{
			Scheme: "ws", Host: "ws-draftkingseu.pusher.com",
			Path:     "app/490c3809b82ef97880f2",
			RawQuery: "protocol=7&client=js&version=4.2.2&flash=false",
		},
		Ping: Ping{
			Interval: time.Second * 6,
			Message: func() string {
				return "{\"event\": \"pusher:ping\", \"data\": \"{}\"}"
			},
		},
		Receive: recv,
		Init: func(send chan []byte) {
			send <- []byte("{\"event\":\"pusher:subscribe\",\"data\":{\"channel\":\"nj_ent-eventgroup-88670847\"}}")
		},
	}
	sub.run()
}
