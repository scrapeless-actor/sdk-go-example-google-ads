package main

import (
	"context"
	"fmt"
	"github.com/scrapeless-ai/scrapeless-actor-sdk-go/scrapeless"
	proxy2 "github.com/scrapeless-ai/scrapeless-actor-sdk-go/scrapeless/proxy"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
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

func main() {
	actor := scrapeless.New(scrapeless.WithProxy(), scrapeless.WithStorage())
	defer actor.Close()
	var param = &RequestParam{}
	if err := actor.Input(param); err != nil {
		log.Error(err)
		panic(err)
	}
	// Get proxy
	proxy, err := actor.Proxy.Proxy(context.TODO(), proxy2.ProxyActor{
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
	// get data
	data, err := getCookie(context.Background(), param)
	if err != nil {
		log.Error(err)
		return
	}
	// Store data
	// You can use bucketId to store data in a specific Object
	// Example:
	// 			sl.Storage.GetObject("bucketId").Put(...)

	objectId, err := actor.Storage.GetObject().Put(context.TODO(), "coockie.json", []byte(data))
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
	request, err := http.NewRequest(http.MethodGet, url, nil)
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
