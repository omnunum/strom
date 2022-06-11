package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	ua "github.com/wux1an/fake-useragent"
)

type Featured struct {
	FeaturedDisplayGroups []struct {
		Name                  string `json:"name"`
		DisplayGroupID        int    `json:"displayGroupId"`
		FeaturedSubcategories []struct {
			SubcategoryName                 string `json:"subcategoryName"`
			SubcategoryID                   int    `json:"subcategoryId"`
			FeaturedEventGroupSubcategories []struct {
				EventGroupID   int    `json:"eventGroupId"`
				EventGroupName string `json:"eventGroupName"`
				Name           string `json:"name"`
				SubcategoryID  int    `json:"subcategoryId"`
				ComponentID    int    `json:"componentId"`
				Offers         [][]struct {
					ProviderOfferID       string `json:"providerOfferId"`
					ProviderID            int    `json:"providerId"`
					ProviderEventID       string `json:"providerEventId"`
					ProviderEventGroupID  string `json:"providerEventGroupId"`
					Label                 string `json:"label"`
					IsSuspended           bool   `json:"isSuspended"`
					IsOpen                bool   `json:"isOpen"`
					OfferSubcategoryID    int    `json:"offerSubcategoryId"`
					IsSubcategoryFeatured bool   `json:"isSubcategoryFeatured"`
					BetOfferTypeID        int    `json:"betOfferTypeId"`
					DisplayGroupID        int    `json:"displayGroupId"`
					EventGroupID          int    `json:"eventGroupId"`
					ProviderCriterionID   string `json:"providerCriterionId"`
					Outcomes              []struct {
						ProviderOutcomeID  string  `json:"providerOutcomeId"`
						ProviderID         int     `json:"providerId"`
						ProviderOfferID    string  `json:"providerOfferId"`
						Label              string  `json:"label"`
						OddsAmerican       string  `json:"oddsAmerican"`
						OddsDecimal        float64 `json:"oddsDecimal"`
						OddsDecimalDisplay string  `json:"oddsDecimalDisplay"`
						OddsFractional     string  `json:"oddsFractional"`
						Line               float64 `json:"line"`
						Main               bool    `json:"main"`
					} `json:"outcomes"`
					OfferUpdateState string `json:"offerUpdateState"`
					OfferSequence    int    `json:"offerSequence"`
					Source           string `json:"source"`
					Main             bool   `json:"main"`
				} `json:"offers"`
			} `json:"featuredEventGroupSubcategories,omitempty"`
			Events []struct {
				EventID         int       `json:"eventId"`
				DisplayGroupID  int       `json:"displayGroupId"`
				EventGroupID    int       `json:"eventGroupId"`
				EventGroupName  string    `json:"eventGroupName"`
				ProviderEventID string    `json:"providerEventId"`
				ProviderID      int       `json:"providerId"`
				Name            string    `json:"name"`
				StartDate       time.Time `json:"startDate"`
				TeamName1       string    `json:"teamName1"`
				TeamName2       string    `json:"teamName2"`
				TeamShortName1  string    `json:"teamShortName1"`
				TeamShortName2  string    `json:"teamShortName2"`
				EventStatus     struct {
					State           string `json:"state"`
					IsClockDisabled bool   `json:"isClockDisabled"`
					Period          string `json:"period"`
					Minute          int    `json:"minute"`
					Second          int    `json:"second"`
					IsClockRunning  bool   `json:"isClockRunning"`
					HomeTeamScore   string `json:"homeTeamScore"`
					AwayTeamScore   string `json:"awayTeamScore"`
				} `json:"eventStatus"`
				EventScorecard struct {
					MainScorecard struct {
						IntervalName    string `json:"intervalName"`
						IntervalNumber  int64  `json:"intervalNumber"`
						FirstTeamScore  string `json:"firstTeamScore"`
						SecondTeamScore string `json:"secondTeamScore"`
					} `json:"mainScorecard"`
					ScoreCards []struct {
						IntervalName    string `json:"intervalName"`
						IntervalNumber  int    `json:"intervalNumber"`
						FirstTeamScore  string `json:"firstTeamScore"`
						SecondTeamScore string `json:"secondTeamScore"`
					} `json:"scoreCards"`
					ScorecardComponentID int `json:"scorecardComponentId"`
				} `json:"eventScorecard"`
				MediaList []struct {
					MediaProviderName string    `json:"mediaProviderName"`
					MediaID           string    `json:"mediaId"`
					MediaTypeName     string    `json:"mediaTypeName"`
					UpdatedAt         time.Time `json:"updatedAt"`
				} `json:"mediaList"`
				LiveBettingOffered bool     `json:"liveBettingOffered"`
				LiveBettingEnabled bool     `json:"liveBettingEnabled"`
				Tags               []string `json:"tags"`
				FlashBetOfferCount int      `json:"flashBetOfferCount"`
			} `json:"events,omitempty"`
		} `json:"featuredSubcategories,omitempty"`
		NumLiveEvents int `json:"numLiveEvents"`
	} `json:"featuredDisplayGroups"`
	TotalLiveEvents int `json:"totalLiveEvents"`
}

type DraftKings struct {
	Client http.Client
}

func (dk DraftKings) GetStreamUrls() ([]url.URL, error) {
	req := http.Request{
		URL: &url.URL{
			Scheme:   "https",
			Host:     "sportsbook.draftkings.com",
			Path:     "/sites/US-SB/api/v3/featured/live",
			RawQuery: "format=json",
		},
		Header: http.Header{
			"User-Agent": []string{ua.RandomType(ua.Desktop)},
		},
	}
	fmt.Println("{}", req.URL.String())
	res, err := dk.Client.Do(&req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	path := strings.Join([]string{
		"featuredDisplayGroups",
		".#.featuredSubcategories",
		".#.featuredEventGroupSubcategories",
		".#.eventGroupId",
	}, "",
	)
	eventGroupIds := gjson.Get(string(body), path).Array()
	// uniqueIDs := Unique[gjson.Result](eventGroupIds)
	// TODO Extract all eventGroupId and return formatted subscription messages
	fmt.Println("{}\n\n{}\n{}", string(body), path, eventGroupIds)

	return make([]url.URL, 0), nil
}
