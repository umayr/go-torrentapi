package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/umayr/go-torrentapi"
)

// flags
var (
	ranked = flag.Bool("ranked", true, "Should results be ranked")
	tvdbid = flag.String("tvdb", "", "TheTVDB ID to search")
	imdb   = flag.String("imdb", "", "The IMDB ID to search")
	search = flag.String("search", "", "Search string")
	sort   = flag.String("sort", "seeders", "Sort order (seeders, leechers, last)")
	limit  = flag.Int("limit", 25, "Limit of results (25, 50, 100)")
)

func humanizeSize(s uint64) string {
	size := float64(s)
	switch {
	case size < 1024:
		return fmt.Sprintf("%d", uint64(size))
	case size < 1024*1014:
		return fmt.Sprintf("%.2fk", size/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.2fM", size/1024/1024)
	default:
		return fmt.Sprintf("%.2fG", size/1024/1024/1024)
	}
}

func main() {
	flag.Parse()
	if *tvdbid == "" && *search == "" && *imdb == "" {
		flag.PrintDefaults()
		return
	}
	api, err := torrentapi.New()
	if err != nil {
		fmt.Printf("Error while querying torrentapi %s", err)
		return
	}
	if *tvdbid != "" {
		api.SearchTVDB(*tvdbid)
	}
	if *imdb != "" {
		api.SearchImDB(*imdb)
	}
	if *search != "" {
		api.SearchString(*search)
	}
	api.Ranked(*ranked).Sort(*sort).Format("json_extended").Limit(*limit)
	results, err := api.Search()
	if err != nil {
		fmt.Printf("Error while querying torrentapi %s", err)
		return
	}
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 1, '\t', 0)

	fmt.Fprintln(w, "File Name\tCategory\tSeeders\tLeechers\tRanked\tSize")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%s\n", r.Title, r.Category, r.Seeders, r.Leechers, r.Ranked, humanizeSize(r.Size))
	}
	w.Flush()
}
