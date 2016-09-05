// torrentapi provides simple and easy Golang interface for RARBG Torrent API v2 (https://torrentapi.org)
package torrentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Supported torrentapi version.
	Version = 2.0

	// Base API URL.
	APIURL = "https://torrentapi.org/pubapi_v2.1.php?"

	// Token expiration time (default is 15 min, but let's expire it after 890 seconds just to be safe.
	TokenExpiration = time.Second * 890
)

// for testing purposes
var apiurl = APIURL

// Token keeps token and it's expiration date.
type Token struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"-"`
}

// EpisodeInfo keepsinformation from "episode_info" key from results. Some of the fields may be empty.
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

// TorrentResults keeps information about single torrent returned from TorrentAPI. Some of the fields may be empty.
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

// API provides interface to access Torrent API.
type API struct {
	Query      string
	APIToken   Token
	categories []int
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

// SearchIMDB dds ImDB id to search query.
func (api *API) SearchImDB(movieid string) *API {
	api.Query += fmt.Sprintf("&search_imdb=%s", movieid)
	return api
}

// SearchMovieDB adds TheMovieDB id to search query.
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
func (api *API) MinSeeders(min_seed int) *API {
	api.Query += fmt.Sprintf("&min_seeders=%d", min_seed)
	return api
}

// MinLeechers specify minimum number of leechers.
func (api *API) MinLeechers(min_leech int) *API {
	api.Query += fmt.Sprintf("&min_leechers=%d", min_leech)
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
func getResults(query string) (*APIResponse, error) {
	resp, err := http.Get(query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var api_response APIResponse
	err = json.NewDecoder(resp.Body).Decode(&api_response)
	return &api_response, err
}

var getRes = getResults

// call calls API and processes response.
func (api *API) call() (data TorrentResults, err error) {
	if !api.APIToken.IsValid() {
		api.APIToken, err = renewToken()
		if err != nil {
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
	query := fmt.Sprintf("%s&token=%s%s", apiurl, api.APIToken.Token, api.Query)
	api_response, err := getRes(query)
	if err != nil {
		return
	}
	data, err = api.processResponse(api_response)
	if err != nil {
		if _, ok := err.(*expiredTokenError); ok {
			// Token expired, renew it and try again
			api.APIToken, err = renewToken()
			if err != nil {
				return
			}
			api_response, err = getRes(query)
			if err != nil {
				return
			}
			data, err = api.processResponse(api_response)
		}
	}
	api.initQuery()
	return
}

type expiredTokenError struct {
	s string
}

func (e expiredTokenError) Error() string {
	return e.s
}

// Process JSON data received from TorrentAPI
func (api *API) processResponse(api_response *APIResponse) (data TorrentResults, err error) {
	if api_response.Torrents != nil {
		// We have valid results
		err = json.Unmarshal(api_response.Torrents, &data)
		if err != nil {
			err = errors.New(fmt.Sprintf("query: %s, Error: %s", api.Query, err.Error()))
		}
	} else if api_response.Error != "" {
		// There was API error
		// Token expired
		if api_response.ErrorCode == 4 {
			return nil, &expiredTokenError{s: "expired token"}
		}
		// No torrents found
		if api_response.ErrorCode == 20 {
			return
		}
		err = errors.New(fmt.Sprintf("query: %s, Error: %s, Error code: %d)", api.Query, api_response.Error, api_response.ErrorCode))
	} else {
		// It shouldn't happen
		err = errors.New(fmt.Sprintf("query: %s, Unknown error: %s", api.Query, err))
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
func RenewToken() (token Token, err error) {
	resp, err := http.Get(apiurl + "get_token=get_token")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&token)
	if err != nil {
		return
	}
	token.Expires = time.Now().Add(TokenExpiration)
	return
}

// for testing
var renewToken = RenewToken

// Init Initializes API object, fetches new token and returns API instance.
func Init() (*API, error) {
	token, err := renewToken()
	if err != nil {
		return nil, err
	}
	api := new(API)
	api.APIToken = token
	api.initQuery()
	return api, err
}
