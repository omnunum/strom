package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/imdario/mergo"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
	ua "github.com/wux1an/fake-useragent"
)

// More efficient to precompile this once upfront
var strip = regexp.MustCompile(`\s`)

type Book struct {
	Sports map[string]*Sport `json:"sports"`
}

type Sport struct {
	Name       string              `json:"name"`
	ID         json.Number         `json:"id"`
	Categories map[string]Category `json:"categories"`
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
			format := `{"event":"pusher:subscribe","data":{"channel":"nj_ent-eventgroup-%s"}}`
			subs = append(subs, fmt.Sprintf(format, category.ID))
		}
	}
	return subs
}

type DraftKings struct {
	Client http.Client
	Book   Book
}

func makeAPIRequest(path string) http.Request {
	return http.Request{
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

func (dk DraftKings) GetAPIResponse(path string) ([]byte, error) {
	req := makeAPIRequest(path)
	res, err := dk.Client.Do(&req)
	urlLog := log.With().Str("path", req.URL.String()).Logger()

	if err != nil {
		urlLog.Error().Err(err).Msg("Requesting API Response")
		return nil, err
	}
	urlLog.Info().Msg("Requested API Response")

	defer res.Body.Close()

	if body, err := io.ReadAll(res.Body); err != nil {
		urlLog.Error().Err(err).Msg("Reading API Response")
		return nil, err
	} else {
		urlLog.Info().Msg("Read API Response")
		urlLog.Debug().RawJSON("response", body).Msg("Read API Response")
		return body, nil
	}
}

type CrawlFrame struct {
	// Passes response through this function in case we need to process
	// it prior to calling the crawling Getter.  Can also be used for
	// caching/outputting the raw response.
	ResponsePreprocessor func(content []byte) ([]byte, error)
	//JSON Path Expression for searching for records to crawl
	Getter string
	// Function that is called on each item found by the Getter
	ItemProcessor func(item gjson.Result) (gjson.Result, error)
	// Turns the item into an API path that can be called for the
	// next Frame to process it's results
	NextPathFormatter func(item gjson.Result) string
}

func (dk DraftKings) Crawl(
	frames []CrawlFrame,
	startPath string,
	results chan gjson.Result,
) *sync.WaitGroup {
	// We need to use a wait group since the number of urls
	// to be scraped is not known until we reach the end
	var wg sync.WaitGroup
	// Define closure first so we can recurse into itself
	var crawl func(int, string)

	crawl = func(step int, path string) {
		defer wg.Done()

		frame := frames[step]
		pathLog := log.With().Str("path", path).Logger()
		body, err := dk.GetAPIResponse(path)
		// Preprocess data in place
		if frame.ResponsePreprocessor != nil {
			body, err = frame.ResponsePreprocessor(body)
			if err != nil {
				pathLog.Error().Err(err).Msg("Preprocessing API response")
			}
			pathLog.Debug().Msg("Preprocessed API response")
		}
		// This allows us to have whitespace in the path
		// when specifying, without breaking GJSON
		getterStripped := strip.ReplaceAllString(frame.Getter, "")
		// Get the item(s) located at the JSON Path
		found := gjson.GetBytes(body, getterStripped)
		for _, v := range found.Array() {
			// In case we want to transform the item before returning
			// as result or crawling any further
			if frame.ItemProcessor != nil {
				v, err = frame.ItemProcessor(v)
				if err != nil {
					pathLog.Error().Err(err).Str("item", v.String()).Msg("Processing item")
				}
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

func (dk *DraftKings) BuildBook() error {
	if dk.Book.Sports == nil {
		dk.Book.Sports = make(map[string]*Sport)
	}
	// Converts an array of objects into an object of objects
	// where the specified arg is the key used to map the inner
	// objects as values of the outer object
	// [{"name": "john", "id":0}, {"name": "mike", "id": 1}]|@objectify:name
	// {"john": {"name": "john", "id":0}, "mike":{"name": "mike", "id": 1}}
	gjson.AddModifier("objectify", func(json, keyPath string) string {
		res := gjson.Parse(json)
		if !res.IsArray() {
			return ""
		}
		data := []byte{'{'}
		var key string
		res.ForEach(func(_, value gjson.Result) bool {
			if !value.IsObject() {
				return true
			}
			if keyPath == "" {
				key = value.Get("@values.0").Raw
			} else {
				key = value.Get(keyPath).Raw
			}
			data = append(data, (key + ":" + value.Raw + ",")...)
			return true
		})
		// Reslice to remove last byte containing trailing comma
		data = data[:len(data)-1]
		data = append(data, '}')
		return string(data)
	})

	frames := []CrawlFrame{{
		Getter: "featuredDisplayGroups.#.displayGroupId",
		NextPathFormatter: func(id gjson.Result) string {
			return fmt.Sprintf("featured/displaygroups/%v/live", id)
		},
	}, {
		Getter: "featuredDisplayGroup.featuredSubcategories.#.subcategoryId",
		NextPathFormatter: func(id gjson.Result) string {
			return fmt.Sprintf("featured/subcategories/%v/live", id)
		},
	}, {
		Getter: fmt.Sprintf(`
			{
				"name":%[1]s.name,
				"id":%[1]s.displayGroupId,
				"categories":{
					"name":%[1]s.featuredSubcategories.#.subcategoryName,
					"id":%[1]s.featuredSubcategories.#.subcategoryId,
				}|@group|@objectify:name
			}`,
			// The responses to this api are odd in that they contain
			// "header" name and ID objects for all the featuredDisplayGroups
			// even if they don't contain any events in our queried subcategory
			// so we filter them out by only selecting groups with subcategories.
			// The bare # gives us the length of the array if it exists at all
			"featuredDisplayGroups.#(featuredSubcategories.#>0)",
		),
	},
	}
	results := make(chan gjson.Result)
	wg := dk.Crawl(frames, "featured/live", results)

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
