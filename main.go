package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/scrapeless-ai/scrapeless-actor-sdk-go/scrapeless"
	proxyModel "github.com/scrapeless-ai/scrapeless-actor-sdk-go/scrapeless/proxy"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type RequestParam struct {
	AdvertiserId   string `json:"advertiser_id"`
	Text           string `json:"text"`
	Region         string `json:"region"`
	Platform       string `json:"platform"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	CreativeFormat string `json:"creative_format"`
	Num            string `json:"num"`
	NextPageToken  string `json:"next_page_token"`
	PoliticalAds   string `json:"political_ads"`
}

var (
	client *http.Client
)

var (
	PlatformMapping = map[string]int{
		"PLAY":     1,
		"MAPS":     2,
		"SEARCH":   3,
		"SHOPPING": 4,
		"YOUTUBE":  5,
	}

	creativeFormatMapping = map[string]int{
		"text":  1,
		"image": 2,
		"video": 3,
	}
)

func main() {
	actor := scrapeless.New(scrapeless.WithProxy(), scrapeless.WithStorage())
	defer actor.Close()
	var param = &RequestParam{}
	if err := actor.Input(param); err != nil {
		log.Error(err)
		panic(err)
	}
	// Get proxy
	proxy, err := actor.Proxy.Proxy(context.TODO(), proxyModel.ProxyActor{
		Country:         "us",
		SessionDuration: 10,
	})
	if err != nil {
		panic(err)
	}
	parse, err := url.Parse(proxy)
	if err != nil {
		panic(err)
	}
	// Set up proxy using Golang's native HTTP
	client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(parse)}}

	// crawl services
	doCrawl(actor, param)
}

func doCrawl(actor *scrapeless.Actor, param *RequestParam) {
	var (
		region       = "anywhere"
		adsResultAll AdsResultAll
	)
	if param.Region != "" {
		region = param.Region
	}
	// get data
	data, err := getCookie(context.Background(), param)
	if err != nil {
		log.Error(err)
		return
	}
	body, err := buildForm(*param)

	urlStr := "https://adstransparency.google.com/anji/_/rpc/SearchService/SearchCreatives"
	urlForm := &url.Values{}
	urlForm.Set("f.req", body)
	request, err := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(urlForm.Encode()))
	if err != nil {
		log.Error(err)
		return
	}
	request.Header.Set("accept", "*/*")
	request.Header.Set("Content-Length", "*/*")
	request.Header.Set("Host", "*/*")
	request.Header.Set("Connection", "keep-alive")
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	request.Header.Set("origin", "https://adstransparency.google.com")
	// 需要动态改
	request.Header.Set("referer", fmt.Sprintf("https://adstransparency.google.com/advertiser/%s?region=%s", param.AdvertiserId, region))
	request.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	request.Header.Set("cookie", data)
	do, err := client.Do(request)
	if err != nil {
		log.Error(err)
		return
	}
	all, _ := io.ReadAll(do.Body)
	defer do.Body.Close()
	results, nextToken, totalCount := parseData(string(all))
	for i, result := range results {
		detailsLink := buildDetailsLink(*param, result)
		results[i].DetailsLink = detailsLink
		if results[i].Format == "" {
			results[i].Format = "text"
		} else {
			results[i].Format = param.CreativeFormat
		}
	}
	resp := []AdsResult{}
	if param.PoliticalAds == "true" {
		for _, result := range results {
			if result.Political {
				resp = append(resp, result)
			}
		}
	} else {
		resp = results
	}
	adsResultAll.AdCreatives = resp
	adsResultAll.NextPageToken = nextToken
	adsResultAll.TotalResults = totalCount
	resultBytes, _ := json.Marshal(adsResultAll)
	// Store data
	// You can use bucketId to store data in a specific Object
	// Example:
	// 			sl.Storage.GetObject("bucketId").Put(...)
	log.Info(string(resultBytes))
	objectId, err := actor.Storage.GetObject().Put(context.TODO(), fmt.Sprintf("ads-%s.json", param.AdvertiserId), resultBytes)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info(objectId)
}

func getCookie(ctx context.Context, info *RequestParam) (string, error) {
	//... Logic
	var (
		err    error
		region = "anywhere"
		cookie string
	)
	if info.Region != "" {
		region = info.Region
	}
	if info.AdvertiserId != "" {
		cookie, err = httpGetCookie(ctx, fmt.Sprintf("https://adstransparency.google.com/advertiser/%s?region=%s", info.AdvertiserId, region))
		if err != nil {
			log.Error(err)
			return "", err
		}
	} else {
		cookie, err = httpGetCookie(ctx, fmt.Sprintf("https://adstransparency.google.com/?region=%s&domain=%s", region, info.Text))
		if err != nil {
			log.Error(err)
			return "", err
		}
	}
	return cookie, nil
}
func httpGetCookie(ctx context.Context, url string) (scrapeRes string, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("request err:%v", err)
		return "", err
	}
	request.Header = http.Header{
		"Accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/jpeg,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"Accept-Language":           {"en-US,en;q=0.9"},
		"Cache-Control":             {"no-cache"},
		"Pragma":                    {"no-cache"},
		"Priority":                  {"u=0, i"},
		"Sec-Ch-Ua":                 {`"Not A(Brand";v="8", "Chromium";v="132", "Google Chrome";v="132"`},
		"Sec-Ch-Ua-Mobile":          {"?0"},
		"Sec-Ch-Ua-Platform":        {`"Windows"`},
		"Sec-Fetch-Dest":            {"document"},
		"Sec-Fetch-Mode":            {"navigate"},
		"Sec-Fetch-Site":            {"none"},
		"Sec-Fetch-User":            {"?1"},
		"Upgrade-Insecure-Requests": {"1"},
		"User-Agent": {
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36",
		},
	}
	resp, err := client.Do(request)
	if err != nil {
		log.Printf("request err:%v", err)
		return "", err
	}
	return resp.Header.Get("Set-Cookie"), nil
}

func buildForm(ads RequestParam) (string, error) {
	num, _ := strconv.Atoi(ads.Num)
	data := make(map[string]any)
	if num == 0 {
		num = 40
	}
	data["2"] = num
	data3 := make(map[string]any)
	region := strings.Split(ads.Region, ",")
	regionInt := []int{}
	for _, s := range region {
		if s == "" {
			continue
		}
		atoi, _ := strconv.Atoi(s)
		regionInt = append(regionInt, atoi)
	}
	if len(regionInt) != 0 {
		data3["8"] = regionInt
	}
	if ads.Platform != "" {
		platform := strings.Split(ads.Platform, ",")
		platformArray := []int{}
		for _, s := range platform {
			if _, ok := PlatformMapping[s]; !ok {
				return "", errors.New("参数异常")
			}
			platformArray = append(platformArray, PlatformMapping[s])
		}

		data3["14"] = platformArray
	}
	if ads.CreativeFormat != "" {
		if _, ok := creativeFormatMapping[ads.CreativeFormat]; !ok {
			return "", errors.New("参数异常")
		}
		data3["4"] = creativeFormatMapping[ads.CreativeFormat]
	}
	if ads.NextPageToken != "" {
		data["4"] = ads.NextPageToken
	}
	if ads.StartDate != "" && ads.EndDate != "" {
		start, err := time.Parse("20060102", ads.StartDate)
		if err != nil {
			return "", err
		}
		end, err := time.Parse("20060102", ads.EndDate)
		if err != nil {
			return "", err
		}
		if !start.Before(end) {
			return "", err
		}
		startDate, _ := strconv.Atoi(ads.StartDate)
		endDate, _ := strconv.Atoi(ads.EndDate)
		//date := map[string]int{"6": startDate, "7": endDate}
		data3["6"] = startDate
		data3["7"] = endDate
	}
	if ads.AdvertiserId != "" {
		advertiserIds := strings.Split(ads.AdvertiserId, ",")
		data3["13"] = map[string]any{
			"1": advertiserIds,
		}
	}
	if ads.Text != "" {
		data3["12"] = map[string]any{
			"1": ads.Text,
			"2": true,
		}
	} else {
		data3["12"] = map[string]any{
			"1": "",
			"2": true,
		}
	}
	data["3"] = data3
	data["7"] = map[string]int{
		"1": 1,
		"2": 0,
		"3": 2156,
	}
	marshal, _ := json.Marshal(data)
	return string(marshal), nil
}

type AdsResultAll struct {
	AdCreatives   []AdsResult `json:"ad_creatives"`
	TotalResults  int64       `json:"total_results,omitempty"`
	NextPageToken string      `json:"next_page_token,omitempty"`
}

type AdsResult struct {
	AdvertiserId       string `json:"advertiser_id"`
	Advertiser         string `json:"advertiser"`
	AdCreativeId       string `json:"ad_creative_id"`
	Format             string `json:"format"` // 这个值与入参creative_format保持一致
	Image              string `json:"image,omitempty"`
	Width              int64  `json:"width,omitempty"`
	Height             int64  `json:"height,omitempty"`
	TotalDaysShown     int64  `json:"total_days_shown"`
	FirstShown         int64  `json:"first_shown"`
	LastShown          int64  `json:"last_shown"`
	MinimumViewsCount  int64  `json:"minimum_views_count,omitempty"`
	MaximumViewsCount  int64  `json:"maximum_views_count,omitempty"`
	MiniMumBudgetSpent string `json:"mini_mum_budget_spent,omitempty"`
	MaxMumBudgetSpent  string `json:"max_mum_budget_spent,omitempty"`
	DetailsLink        string `json:"details_link"`
	Political          bool   `json:"-"`
}

func parseData(data string) ([]AdsResult, string, int64) {
	parse := gjson.Parse(data)
	nextToken := parse.Get("2").String()
	m := parse.Map()
	countIndex := 0
	for s, _ := range m {
		maxI, _ := strconv.Atoi(s)
		if maxI > countIndex {
			countIndex = maxI
		}
	}
	totalCount := parse.Get(fmt.Sprintf("%d", countIndex)).Int()

	results := parse.Get("1").Array()
	adsResults := make([]AdsResult, 0)
	for _, r := range results {
		advertiserId := r.Get("1").String()
		adCreativeId := r.Get("2").String()
		advertiser := r.Get("12").String()
		imagesDom := r.Get("3.3.2").String()
		dc, _ := goquery.NewDocumentFromReader(strings.NewReader(imagesDom))
		img := dc.Find("img")
		image, _ := img.Attr("src")
		heightStr, _ := img.Attr("height")
		height, _ := strconv.Atoi(heightStr)
		widthStr, _ := img.Attr("width")
		width, _ := strconv.Atoi(widthStr)

		firstShown := r.Get("6.1").Int()
		lastShown := r.Get("7.1").Int()
		totalDaysShown := r.Get("13").Int()
		minimumViewsCount := r.Get("8").Int()
		maxmumViewsCount := r.Get("9").Int()
		result := AdsResult{
			AdvertiserId:      advertiserId,
			Advertiser:        advertiser,
			AdCreativeId:      adCreativeId,
			Image:             image,
			Width:             int64(width),
			Height:            int64(height),
			TotalDaysShown:    totalDaysShown,
			FirstShown:        firstShown,
			LastShown:         lastShown,
			MinimumViewsCount: minimumViewsCount,
			MaximumViewsCount: maxmumViewsCount,
		}
		if r.Get("10.1").Exists() {
			miniMumBudgetSpent := r.Get("10.1").Array()[0].Get("2").String() + " " + r.Get("10.1").Array()[0].Get("1").String()
			result.MiniMumBudgetSpent = miniMumBudgetSpent
		}
		if r.Get("11.1").Exists() {
			maxMumBudgetSpent := r.Get("11.1").Array()[0].Get("2").String() + " " + r.Get("11.1").Array()[0].Get("1").String()
			result.MaxMumBudgetSpent = maxMumBudgetSpent
		}
		if r.Get("15").Exists() {
			result.Political = true
		}
		adsResults = append(adsResults, result)
	}
	return adsResults, nextToken, totalCount
}

func buildDetailsLink(info RequestParam, r AdsResult) string {
	resp := fmt.Sprintf("https://adstransparency.google.com/advertiser/%s/creative/%s?region=%s", r.AdvertiserId, r.AdCreativeId, info.Region)
	if info.CreativeFormat != "" {
		resp += fmt.Sprintf("&format=%s", strings.ToUpper(info.CreativeFormat))
	}
	if info.StartDate != "" && info.EndDate != "" {
		start, _ := time.Parse("20060102", info.StartDate)
		end, _ := time.Parse("20060102", info.EndDate)
		resp += fmt.Sprintf("&start-date=%s", start.Format(time.DateOnly))
		resp += fmt.Sprintf("&end-date=%s", end.Format(time.DateOnly))
	}
	if info.Platform != "" {
		resp += fmt.Sprintf("&platform=%s", strings.ToUpper(info.Platform))
	}
	return resp
}
