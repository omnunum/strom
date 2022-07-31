package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
)

type Crawler struct {
	Client http.Client
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
	// next Frame to process it's results.  Outputs a list in case
	// the item needs to be "fanned out" into multiple requests
	NextReqFormatter func(item gjson.Result) ([]*http.Request, error)
}

// More efficient to precompile this once upfront
var strip = regexp.MustCompile(`[\t\n]`)

func (c Crawler) getAPIResponse(req *http.Request) ([]byte, error) {
	urlLog := log.With().Str("path", req.URL.String()).Logger()
	res, err := c.Client.Do(req)
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

func (c Crawler) Crawl(
	frames []CrawlFrame,
	startRequest *http.Request,
	results chan gjson.Result,
) *sync.WaitGroup {
	// We need to use a wait group since the number of urls
	// to be scraped is not known until we reach the end
	var wg sync.WaitGroup
	// Define closure first so we can recurse into itself
	var crawl func(int, *http.Request)

	crawl = func(step int, req *http.Request) {
		defer wg.Done()

		frame := frames[step]
		pathLog := log.With().Str("host", req.URL.Host).Str("path", req.URL.String()).Logger()
		body, err := c.getAPIResponse(req)
		// Preprocess data in place if we have specified a function
		if frame.ResponsePreprocessor != nil {
			body, err = frame.ResponsePreprocessor(body)
			if err != nil {
				pathLog.Error().Err(err).Msg("Preprocessing API response")
			}
			pathLog.Debug().Msg("Preprocessed API response")
		}
		// This allows us to have whitespace in the path (more readable)
		// without breaking GJSON paths (they need to be single line)
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
				if frame.NextReqFormatter == nil {
					panic(fmt.Errorf(
						"Frame %d is not the last frame yet it lacks a NextReqFormatter", step,
					))
				}
				reqs, err := frame.NextReqFormatter(v)
				if err != nil {
					pathLog.Error().Err(err).Str("item", v.String()).Msg("Formatting next request for item")
					continue
				}
				for _, nextReq := range reqs {
					wg.Add(1)
					go crawl(step+1, nextReq)
				}

				// Otherwise we've hit the end of the line and want
				// to send the item to the results channel to be collected
			} else {
				results <- v
			}
		}
	}
	wg.Add(1)
	go crawl(0, startRequest)
	return &wg
}

// We need to register some extra functionality with GJSON
func init() {
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
	gjson.AddModifier("wrap", func(json, keyPath string) string {
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
}
