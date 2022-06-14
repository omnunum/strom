package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"github.com/rs/zerolog/log"
	ua "github.com/wux1an/fake-useragent"
)

type DraftKings struct {
	Client http.Client
}

func makeAPIRequest(path string) http.Request {
	return http.Request{
		URL: &url.URL{
			Scheme:   "https",
			Host:     "sportsbook.draftkings.com",
			Path:     "/sites/US-SB/api/v3/" + path,
			RawQuery: "format=json",
		},
		Header: http.Header{
			"User-Agent": []string{ua.RandomType(ua.Desktop)},
		},
	}
}

func (dk DraftKings) GetAPIResponse(path string) ([]byte, error) {
	req := makeAPIRequest(path)
	res, err := dk.Client.Do(&req)
	if err != nil {
		log.Error().Err(err).Str("url", req.URL.String()).Msg("Requesting API Response")
		return nil, err
	}
	log.Info().Str("url", req.URL.String()).Msg("Requested API Response")

	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

type CrawlFrame struct {
	//JSON Path Expression for searching for records to crawl
	Getter jp.Expr
	//
	ItemProcessor     func(item any) any
	NextPathFormatter func(item any) string
}

func (dk DraftKings) Crawl(
	frames []CrawlFrame,
	startPath string,
	results chan any,
) *sync.WaitGroup {
	// We need to use a wait group since the number of urls
	// we need to crawl is unknown to us until they are all done
	var wg sync.WaitGroup
	// Define closure first so we can recurse into itself
	var crawl func(int, string)

	crawl = func(step int, path string) {
		defer wg.Done()

		frame := frames[step]
		body, err := dk.GetAPIResponse(path)
		if err != nil {
			log.Error().Str("path", path).Err(err).Msg("Fetching API response")
			return
		}
		log.Debug().Str("path", path).RawJSON("response", body).Msg("Fetched API response")
		// Parse the JSON response into a traversable object
		parsed, err := oj.Parse(body)
		if err != nil {
			log.Error().Str("path", path).Err(err).Msg("Parsing API response")
			return
		}
		log.Debug().Str("path", path).Msg("Parsed API response")
		// Get the item(s) located at the JSON Path
		found := frame.Getter.Get(parsed)
		for _, v := range found {
			// In case we want to transform the item before returning
			// as result or crawling any further
			if frame.ItemProcessor != nil {
				v = frame.ItemProcessor(v)
			}
			// If there is a further step (frame) in the crawling process
			// then crawl the next path
			if step < len(frames)-1 {
				wg.Add(1)
				go crawl(step+1, frame.NextPathFormatter(v))
				// Otherwise we've hit the end of the line and want
				// to send the item to the results channel to be collected
			} else {
				results <- v
			}
		}
	}
	wg.Add(1)
	go crawl(0, startPath)
	return &wg
}

func (dk DraftKings) GetStreamSubscriptions() ([]string, error) {
	frames := []CrawlFrame{{
		Getter: jp.MustParseString("$.featuredDisplayGroups.*.displayGroupId"),
		NextPathFormatter: func(id any) string {
			return fmt.Sprintf("featured/displaygroups/%d/live", id)
		},
	}, {
		Getter: jp.MustParseString("$.featuredDisplayGroup.featuredSubcategories.*.featuredEventGroupSubcategories.*.eventGroupId"),
	},
	}
	results := make(chan any)
	wg := dk.Crawl(frames, "featured/live", results)

	// close the results channel when we've finished crawling
	go func() {
		defer close(results)
		wg.Wait()
	}()

	subs := make([]string, 0)
	format := `{"event":"pusher:subscribe","data":{"channel":"nj_ent-eventgroup-%d"}}`
	for r := range results {
		subs = append(subs, fmt.Sprintf(format, r))
	}

	return subs, nil
}

func (dk DraftKings) SubscribeToStreams(recv chan []byte) (*sync.WaitGroup, error) {
	wg := sync.WaitGroup{}
	subs, err := dk.GetStreamSubscriptions()
	if err != nil {
		return nil, err
	}

	for _, s := range subs {
		s := s
		sub := Subscription{
			Request: http.Request{
				URL: &url.URL{
					Scheme: "ws", Host: "ws-draftkingseu.pusher.com",
					Path:     "app/490c3809b82ef97880f2",
					RawQuery: "protocol=7&client=js&version=4.2.2&flash=false",
				},
				Header: http.Header{
					"User-Agent": []string{ua.RandomType(ua.Desktop)},
				},
			},
			Ping: Ping{
				Interval: time.Second * 6,
				Message: func() string {
					return `{"event": "pusher:ping", "data": "{}"}`
				},
			},
			Receive: recv,
			Wait:    &wg,
			Init: func(send chan []byte) {
				send <- []byte(s)
			},
		}
		wg.Add(1)
		go sub.run()
	}
	return &wg, nil
}
