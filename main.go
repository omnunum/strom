package main

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	recv := make(chan []byte)

	dk := DraftKings{}
	wg, err := dk.SubscribeToStreams(recv)
	if err != nil {
		log.Error().Err(err)
	}

	// Close the recieving channel when all Subscriptions
	// have finished
	go func() {
		wg.Wait()
		close(recv)
	}()

	for r := range recv {
		log.Info().RawJSON("msg", r).Msg("Recieved")
	}
	log.Info().Msg("Closed all Subscriptions and exited")
}
