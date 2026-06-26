// Package freeproxy — crawler.go: concurrent proxy crawler engine.
package freeproxy

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CrawlResult holds the results of a crawl operation.
type CrawlResult struct {
	Total   int               `json:"total"`
	Sources map[string]int    `json:"sources"` // source name → count
	Entries []ProxyEntry      `json:"-"`
	Errors  []string          `json:"errors"`
}

// Crawl fetches proxies from all enabled sources concurrently.
func Crawl(ctx context.Context, sources []Source, timeout time.Duration) *CrawlResult {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	var (
		mu     sync.Mutex
		result = &CrawlResult{Sources: map[string]int{}}
		seen   = map[string]bool{}
		wg     sync.WaitGroup
	)

	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()

			log.Printf("[crawl] fetching from %s: %s", s.Name, s.URL)
			body, err := FetchSourceBody(s.URL, timeout, s.Headers)
			if err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", s.Name, err))
				mu.Unlock()
				log.Printf("[crawl] %s error: %v", s.Name, err)
				return
			}

			entries, err := ParseSourceResponse(s, body)
			if err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("%s parse: %v", s.Name, err))
				mu.Unlock()
				log.Printf("[crawl] %s parse error: %v", s.Name, err)
				return
			}

			mu.Lock()
			count := 0
			for _, e := range entries {
				key := e.Addr + "|" + e.Type
				if seen[key] {
					continue
				}
				seen[key] = true
				result.Entries = append(result.Entries, e)
				count++
			}
			result.Sources[s.Name] = count
			result.Total += count
			mu.Unlock()

			log.Printf("[crawl] %s: %d proxies", s.Name, count)
		}(src)
	}
	wg.Wait()
	return result
}
