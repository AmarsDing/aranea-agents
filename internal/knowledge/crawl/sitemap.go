package crawl

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// ParseSitemap parses a sitemap XML document (urlset or sitemapindex) and
// returns the page URLs and, for a sitemap index, the nested sitemap URLs.
// Recursion into nested sitemaps is left to the caller: fetch each entry of
// subSitemaps and call ParseSitemap again.
func ParseSitemap(data []byte) (urls []string, subSitemaps []string, err error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil, errors.New("crawl: empty sitemap document")
	}
	var probe struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(trimmed, &probe); err != nil {
		return nil, nil, fmt.Errorf("crawl: parse sitemap: %w", err)
	}
	switch strings.ToLower(probe.XMLName.Local) {
	case "urlset":
		var set struct {
			URLs []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
		}
		if err := xml.Unmarshal(trimmed, &set); err != nil {
			return nil, nil, fmt.Errorf("crawl: parse urlset: %w", err)
		}
		for _, u := range set.URLs {
			if loc := strings.TrimSpace(u.Loc); loc != "" {
				urls = append(urls, loc)
			}
		}
	case "sitemapindex":
		var index struct {
			Sitemaps []struct {
				Loc string `xml:"loc"`
			} `xml:"sitemap"`
		}
		if err := xml.Unmarshal(trimmed, &index); err != nil {
			return nil, nil, fmt.Errorf("crawl: parse sitemapindex: %w", err)
		}
		for _, sm := range index.Sitemaps {
			if loc := strings.TrimSpace(sm.Loc); loc != "" {
				subSitemaps = append(subSitemaps, loc)
			}
		}
	default:
		return nil, nil, fmt.Errorf("crawl: unsupported sitemap root element %q", probe.XMLName.Local)
	}
	return urls, subSitemaps, nil
}
