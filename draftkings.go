package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/imdario/mergo"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
	ua "github.com/wux1an/fake-useragent"
)

type Book struct {
	Sports map[string]*Sport `json:"sports"`
}

type Sport struct {
	Name       string               `json:"name"`
	ID         json.Number          `json:"id"`
	Categories map[string]*Category `json:"categories"`
}

type Category struct {
	Name  string          `json:"name"`
	ID    json.Number     `json:"id"`
	Lines map[string]Line `json:"lines,omitempty"`
}

type Line struct {
	Name string      `json:"name"`
	ID   json.Number `json:"id"`
}

func (b Book) GetEventSubscriptions() []string {
	subs := make([]string, 0)
	for _, sport := range b.Sports {
		for _, category := range sport.Categories {
			format := `{"event":"pusher:subscribe","data":{"channel":"nj_ent-eventgroupv2-%s"}}`
			subs = append(subs, fmt.Sprintf(format, category.ID))
		}
	}
	return subs
}

type DraftKings struct {
	Crawler Crawler
	Book    Book
}

func makeAPIRequest(path string) *http.Request {
	return &http.Request{
		URL: &url.URL{
			Scheme:   "https",
			Host:     "sportsbook.draftkings.com",
			Path:     "/sites/US-SB/api/v4/" + path,
			RawQuery: "format=json",
		},
		Header: http.Header{
			"User-Agent": []string{ua.RandomType(ua.Desktop)},
		},
	}
}

func (dk *DraftKings) BuildBook() error {
	if dk.Book.Sports == nil {
		dk.Book.Sports = make(map[string]*Sport)
	}

	frames := []CrawlFrame{{
		Getter: "featuredDisplayGroups.#.displayGroupId",
		NextReqFormatter: func(id gjson.Result) (reqs []*http.Request, err error) {
			path := fmt.Sprintf("featured/displaygroups/%v/live", id)
			return append(reqs, makeAPIRequest(path)), nil
		},
	}, {
		Getter: `[[	
			featuredDisplayGroup.displayGroupId,
			featuredDisplayGroup.featuredSubcategories.#.subcategoryId
		]]`,
		NextReqFormatter: func(ids gjson.Result) (reqs []*http.Request, err error) {
			idArray := ids.Array()
			groupId, subcategoryIds := idArray[0], idArray[1]
			for _, sub := range subcategoryIds.Array() {
				path := fmt.Sprintf(
					"featured/displaygroups/%v/subcategory/%v/live",
					groupId, sub.String(),
				)
				reqs = append(reqs, makeAPIRequest(path))
			}
			return reqs, nil
		},
	}, {
		Getter: fmt.Sprintf(
			`[[	
				featuredDisplayGroup.displayGroupId,
				%[1]s.subcategoryId,
				%[1]s.featuredEventGroupSubcategories.0.eventGroupId
			]]`,
			// Fetch all subcategories that have at least one event group.
			// In practice this should only be a single element, as all other
			// subcategories not specified in the request should not return
			// any event groups and will be missing the featuredEventGroupSubcategories key
			"featuredDisplayGroup.featuredSubcategories.#(featuredEventGroupSubcategories.#>0)",
		),
		NextReqFormatter: func(ids gjson.Result) (reqs []*http.Request, err error) {
			idArray := ids.Array()
			if len(idArray) < 3 {
				return nil, fmt.Errorf("response had fewer than 3 elements")
			}
			path := fmt.Sprintf(
				"featured/displaygroups/%v/subcategory/%v/eventgroup/%v/live",
				idArray[0], idArray[1], idArray[2],
			)
			return append(reqs, makeAPIRequest(path)), nil
		},
	}, {
		Getter: fmt.Sprintf(`
			{
				"name":%[1]s.name,
				"id":%[1]s.displayGroupId,
				"categories":{
					"name":%[1]s.%[2]s.subcategoryName,
					"id":%[1]s.%[2]s.subcategoryId,
				}|@group|@objectify:name
			}`,
			// The responses to this api are odd in that they contain
			// "header" name and ID objects for all the featuredDisplayGroups
			// even if they don't contain any events in our queried subcategory
			// so we filter them out by only selecting groups with subcategories.
			// The bare # gives us the length of the array if it exists at all
			"featuredDisplayGroup", "featuredSubcategories.#(featuredEventGroupSubcategories.#>0)#",
		),
	},
	}
	results := make(chan gjson.Result)
	wg := dk.Crawler.Crawl(frames, makeAPIRequest("featured/live"), results)

	// close the results channel when we've finished crawling
	go func() {
		defer close(results)
		wg.Wait()
	}()

	for r := range results {
		raw := []byte(r.String())
		sport := &Sport{}

		// TODO: compress this pattern of logging an action as Debug or Error
		// into a function call using a zerologger context and replacing the
		// "ed"/"ing" verb Msg with a suffix-free verb
		// Msg("Preprocessed API response") -> .String("verb", "Preprocess").String("subject", "API Response")

		if err := json.Unmarshal(raw, sport); err != nil {
			log.Error().Err(err).RawJSON("rawResponse", raw).Msg("Unmarshalling")
		}
		log.Debug().RawJSON("rawResponse", raw).Msg("Unmarshalled")

		if existing, ok := dk.Book.Sports[sport.Name]; ok {
			mergo.MergeWithOverwrite(existing, sport)
		} else {
			dk.Book.Sports[sport.Name] = sport
		}
	}

	return nil
}

func (dk DraftKings) SubscribeToStreams(recv chan []byte) (*sync.WaitGroup, error) {
	wg := sync.WaitGroup{}
	subs := dk.Book.GetEventSubscriptions()

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
