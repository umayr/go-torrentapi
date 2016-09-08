// Package torrentapi provides simple and easy Golang interface for RARBG Torrent API v2 (https://torrentapi.org)
package torrentapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Supported torrentapi version.
	version = "v2"

	// Base API URL.
	apiURL = "https://torrentapi.org/pubapi_%s.php?"

	// Token expiration time (default is 15 min, but let's expire it after 890 seconds just to be safe.
	tokenExpiration = time.Second * 890

	// Error code API returns when token has expired
	errCodeTokenExpired = 4

	// Error code API returns when there's no torrents to show
	errCodeNoTorrents = 20
)

// Token keeps token and it's expiration date.
type Token struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"-"`
}

// IsValid Check if token is still valid.
func (t *Token) IsValid() bool {
	if t.Token == "" {
		return false
	}
	if time.Now().After(t.Expires) {
		return false
	}
	return true
}

// EpisodeInfo keeps information from "episode_info" key from results. Some of the fields may be empty.
type EpisodeInfo struct {
	ImDB       string `json:"imdb"`
	TvDB       string `json:"tvdb"`
	TvRage     string `json:"tvrage"`
	TheMovieDB string `json:"themoviedb"`
	AirDate    string `json:"airdate"`
	SeasonNum  string `json:"seasonnum"`
	EpisodeNum string `json:"epnum"`
	Title      string `json:"title"`
}

// TorrentResult keeps information about single torrent returned from TorrentAPI. Some of the fields may be empty.
type TorrentResult struct {
	Title       string      `json:"title"`
	Filename    string      `json:"filename"`
	Category    string      `json:"category"`
	Download    string      `json:"download"`
	Seeders     int         `json:"seeders"`
	Leechers    int         `json:"leechers"`
	Size        uint64      `json:"size"`
	PubDate     string      `json:"pubdate"`
	Ranked      int         `json:"ranked"`
	InfoPage    string      `json:"info_page"`
	EpisodeInfo EpisodeInfo `json:"episode_info"`
}

// TorrentResults represents multiple results.
type TorrentResults []TorrentResult

// APIResponse from Torrent API.
type APIResponse struct {
	Torrents  json.RawMessage `json:"torrent_results"`
	Error     string          `json:"error"`
	ErrorCode int             `json:"error_code"`
}

type expiredTokenError struct {
	s string
}

func (e expiredTokenError) Error() string {
	return e.s
}

// Config for API instance
type Config struct {
	Version         string
	TokenExpiration time.Duration
	Client          *http.Client
}

// API provides interface to access Torrent API.
type API struct {
	Query    string
	APIToken Token

	categories []int

	apiURL          string
	fetch           func(string) (*http.Response, error)
	tokenExpiration time.Duration
}

// SearchString adds search string to search query.
func (api *API) SearchString(query string) *API {
	api.Query += fmt.Sprintf("&search_string=%s", url.QueryEscape(query))
	return api
}

// Category adds category to search query.
func (api *API) Category(category int) *API {
	api.categories = append(api.categories, category)
	return api
}

// SearchTVDB adds TheTVDB id to search query.
func (api *API) SearchTVDB(seriesid string) *API {
	api.Query += fmt.Sprintf("&search_tvdb=%s", seriesid)
	return api
}

// SearchImDB dds ImDB id to search query.
func (api *API) SearchImDB(movieid string) *API {
	api.Query += fmt.Sprintf("&search_imdb=%s", movieid)
	return api
}

// SearchTheMovieDB adds TheMovieDB id to search query.
func (api *API) SearchTheMovieDB(movieid string) *API {
	api.Query += fmt.Sprintf("&search_themoviedb=%s", movieid)
	return api
}

// Format requests different results format, possible values json, json_extended. Please note that whith json format not all fields are populated in TorrentResult.
func (api *API) Format(format string) *API {
	api.Query += fmt.Sprintf("&format=%s", format)
	return api
}

// Limit adds limit to number of results.
func (api *API) Limit(limit int) *API {
	api.Query += fmt.Sprintf("&limit=%d", limit)
	return api
}

// Sort results based on seeders, leechers or last(default).
func (api *API) Sort(sort string) *API {
	api.Query += fmt.Sprintf("&sort=%s", sort)
	return api
}

// Ranked sets if returned results should be ranked.
func (api *API) Ranked(ranked bool) *API {
	if ranked {
		api.Query += "&ranked=1"
	} else {
		api.Query += "&ranked=0"
	}
	return api
}

// MinSeeders specify minimum number of seeders.
func (api *API) MinSeeders(minSeed int) *API {
	api.Query += fmt.Sprintf("&min_seeders=%d", minSeed)
	return api
}

// MinLeechers specify minimum number of leechers.
func (api *API) MinLeechers(minLeech int) *API {
	api.Query += fmt.Sprintf("&min_leechers=%d", minLeech)
	return api
}

// List lists the newest torrrents, this has to be last function in chain.
func (api *API) List() (TorrentResults, error) {
	api.Query += "&mode=list"
	return api.call()
}

// Search performs search, this has to be last function in chain.
func (api *API) Search() (TorrentResults, error) {
	api.Query += "&mode=search"
	return api.call()
}

// getResults sends query to TorrentAPI and fetch the response.
func (api *API) getResults(query string) (*APIResponse, error) {
	resp, err := api.fetch(query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r APIResponse
	err = json.NewDecoder(resp.Body).Decode(&r)
	return &r, err
}

// call calls API and processes response.
func (api *API) call() (data TorrentResults, err error) {
	if !api.APIToken.IsValid() {
		if err = api.renewToken(); err != nil {
			return nil, err
		}
	}
	if len(api.categories) > 0 {
		categories := make([]string, len(api.categories))
		for i, c := range api.categories {
			categories[i] = strconv.Itoa(c)
		}
		api.Query += fmt.Sprintf("&category=%s", strings.Join(categories, ";"))
	}
	query := fmt.Sprintf("%s&token=%s%s", api.apiURL, api.APIToken.Token, api.Query)
	r, err := api.getResults(query)
	if err != nil {
		return
	}
	data, err = api.processResponse(r)
	if err != nil {
		if _, ok := err.(*expiredTokenError); ok {
			// Token expired, renew it and try again
			if err = api.renewToken(); err != nil {
				return nil, err
			}
			r, err = api.getResults(query)
			if err != nil {
				return
			}
			data, err = api.processResponse(r)
		}
	}
	api.initQuery()
	return
}

// Process JSON data received from TorrentAPI
func (api *API) processResponse(r *APIResponse) (data TorrentResults, err error) {
	if r.Torrents != nil {
		// We have valid results
		err = json.Unmarshal(r.Torrents, &data)
		if err != nil {
			err = fmt.Errorf("query: %s, Error: %s", api.Query, err.Error())
		}
	} else if r.Error != "" {
		// There was API error
		// Token expired
		if r.ErrorCode == errCodeTokenExpired {
			return nil, &expiredTokenError{s: "expired token"}
		}
		// No torrents found
		if r.ErrorCode == errCodeNoTorrents {
			return
		}
		err = fmt.Errorf("query: %s, Error: %s, Error code: %d)", api.Query, r.Error, r.ErrorCode)
	} else {
		// It shouldn't happen
		err = fmt.Errorf("query: %s, Unknown error: %s", api.Query, err)
	}
	// Clear Query variable
	return data, err
}

// initQuery cleans query state.
func (api *API) initQuery() {
	api.categories = api.categories[:0]
	api.Query = ""
}

// RenewToken fetches new token.
func (api *API) renewToken() (err error) {
	resp, err := api.fetch(api.apiURL + "get_token=get_token")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	t := Token{}
	err = json.NewDecoder(resp.Body).Decode(&t)
	if err != nil {
		return
	}
	t.Expires = time.Now().Add(api.tokenExpiration)
	api.APIToken = t

	return
}

// New initializes API object with default configuration, fetches new token and returns API instance.
func New() (*API, error) {
	api := new(API)
	api.apiURL = fmt.Sprintf(apiURL, version)
	api.fetch = http.Get
	api.tokenExpiration = tokenExpiration

	if err := api.renewToken(); err != nil {
		return nil, err
	}

	api.initQuery()
	return api, nil
}

// NewWithConfig initializes API object with provided configuration, fetches new token and returns API instance.
func NewWithConfig(conf *Config) (*API, error) {
	api := new(API)

	if conf.Version != "" {
		api.apiURL = fmt.Sprintf(apiURL, conf.Version)
	} else {
		api.apiURL = fmt.Sprintf(apiURL, version)
	}

	if conf.Client != nil {
		api.fetch = conf.Client.Get
	} else {
		api.fetch = http.Get
	}

	if conf.TokenExpiration != 0 {
		api.tokenExpiration = conf.TokenExpiration
	} else {
		api.tokenExpiration = tokenExpiration
	}

	if err := api.renewToken(); err != nil {
		return nil, err
	}

	api.initQuery()
	return api, nil
}
