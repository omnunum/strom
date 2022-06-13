package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
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
	fmt.Println(req.URL.String())
	res, err := dk.Client.Do(&req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

type CrawlFrame struct {
	Getter           jp.Expr
	ItemProcessor    func(item any) any
	NextURLFormatter func(item any) string
}

func (dk DraftKings) CrawlFrames(
	frames []CrawlFrame,
	startPath string,
	results chan any,
) *sync.WaitGroup {
	var wg sync.WaitGroup
	var crawl func(int, string)

	crawl = func(step int, path string) {
		defer wg.Done()

		frame := frames[step]
		body, err := dk.GetAPIResponse(path)
		if err != nil {
			return
		}
		parsed, err := oj.Parse(body)
		if err != nil {
			return
		}
		found := frame.Getter.Get(parsed)
		for _, v := range found {
			if step < len(frames)-1 {
				wg.Add(1)
				go crawl(step+1, frame.NextURLFormatter(v))
			} else {
				results <- v
			}
		}
	}
	// start by crawling the first url, which will add the
	// work it finds to the channel
	wg.Add(1)
	go crawl(0, startPath)
	return &wg
}

func (dk DraftKings) GetStreamSubscriptions() ([]string, error) {
	frames := []CrawlFrame{{
		Getter: jp.MustParseString("$.featuredDisplayGroups.*.displayGroupId"),
		NextURLFormatter: func(id any) string {
			return fmt.Sprintf("featured/displaygroups/%s/live", id)
		},
	}, {
		Getter: jp.MustParseString("$.featuredDisplayGroup.featuredSubcategories.*.featuredEventGroupSubcategories.*.eventGroupId"),
	},
	}
	results := make(chan any)
	dk.CrawlFrames(frames, "featured/live", results).Wait()
	close(results)
	subs := make([]string, 0)
	format := `{"event":"pusher:subscribe","data":{"channel":"nj_ent-eventgroup-%s"}}`
	for r := range results {
		subs = append(subs, fmt.Sprintf(format, r))
	}

	return subs, nil
}
