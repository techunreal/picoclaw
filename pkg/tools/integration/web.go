package integrationtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"

	kagiopenapi "github.com/kagisearch/kagi-openapi-golang"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/utils"
)

const (
	userAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	sogouUserAgent  = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1"
	userAgentHonest = "picoclaw/%s (+https://github.com/sipeed/picoclaw; AI assistant bot)"

	// HTTP client timeouts for web tool providers.
	searchTimeout     = 10 * time.Second // Brave, Tavily, DuckDuckGo
	perplexityTimeout = 30 * time.Second // Perplexity (LLM-based, slower)
	fetchTimeout      = 60 * time.Second // WebFetchTool

	defaultMaxChars = 12000
	maxRedirects    = 5
)

// Pre-compiled regexes for HTML text extraction
var (
	reScript     = regexp.MustCompile(`<script[\s\S]*?</script>`)
	reStyle      = regexp.MustCompile(`<style[\s\S]*?</style>`)
	reTags       = regexp.MustCompile(`<[^>]+>`)
	reWhitespace = regexp.MustCompile(`[^\S\n]+`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)

	reSogouTitle = regexp.MustCompile(
		`<a\s+class="?resultLink"?\s+href="([^"]+)"[^>]*id="sogou_vr_\d+_\d+"[^>]*>\s*(.*?)\s*</a>`,
	)
	reSogouSnippet = regexp.MustCompile(`<div class="clamp\d*[^"]*">\s*(.*?)\s*</div>`)
	reSogouRealURL = regexp.MustCompile(`url=([^&]+)`)
)

type APIKeyPool struct {
	keys    []string
	current uint32
}

func NewAPIKeyPool(keys []string) *APIKeyPool {
	return &APIKeyPool{
		keys: keys,
	}
}

type APIKeyIterator struct {
	pool     *APIKeyPool
	startIdx uint32
	attempt  uint32
}

func (p *APIKeyPool) NewIterator() *APIKeyIterator {
	if len(p.keys) == 0 {
		return &APIKeyIterator{pool: p}
	}
	idx := atomic.AddUint32(&p.current, 1) - 1
	return &APIKeyIterator{
		pool:     p,
		startIdx: idx,
	}
}

func (it *APIKeyIterator) Next() (string, bool) {
	length := uint32(len(it.pool.keys))
	if length == 0 || it.attempt >= length {
		return "", false
	}
	key := it.pool.keys[(it.startIdx+it.attempt)%length]
	it.attempt++
	return key, true
}

type SearchProvider interface {
	Search(ctx context.Context, query string, count int, rangeCode string) (string, error)
}

type SearchResultItem struct {
	Title     string
	URL       string
	Snippet   string
	Published string
}

func extractSogouURL(href string) string {
	match := reSogouRealURL.FindStringSubmatch(href)
	if len(match) < 2 {
		return ""
	}
	decoded, err := url.QueryUnescape(match[1])
	if err != nil {
		return ""
	}
	return decoded
}

func applySogouRangeHint(query string, rangeCode string) string {
	switch rangeCode {
	case "d":
		return query + " 最近一天"
	case "w":
		return query + " 最近一周"
	case "m":
		return query + " 最近一个月"
	case "y":
		return query + " 最近一年"
	default:
		return query
	}
}

func normalizeSearchRange(raw string) (string, error) {
	rangeCode := strings.ToLower(strings.TrimSpace(raw))
	switch rangeCode {
	case "", "d", "w", "m", "y":
		return rangeCode, nil
	default:
		return "", fmt.Errorf("range must be one of: d, w, m, y")
	}
}

func mapBraveFreshness(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "pd"
	case "w":
		return "pw"
	case "m":
		return "pm"
	case "y":
		return "py"
	default:
		return ""
	}
}

func mapTavilyTimeRange(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "day"
	case "w":
		return "week"
	case "m":
		return "month"
	case "y":
		return "year"
	default:
		return ""
	}
}

func mapPerplexityRecencyFilter(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "day"
	case "w":
		return "week"
	case "m":
		return "month"
	case "y":
		return "year"
	default:
		return ""
	}
}

func mapDuckDuckGoDateFilter(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "d"
	case "w":
		return "w"
	case "m":
		return "m"
	case "y":
		return "t"
	default:
		return ""
	}
}

func mapSearXNGTimeRange(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "day"
	case "w":
		return "week"
	case "m":
		return "month"
	case "y":
		return "year"
	default:
		return ""
	}
}

func mapGLMRecencyFilter(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "oneDay"
	case "w":
		return "oneWeek"
	case "m":
		return "oneMonth"
	case "y":
		return "oneYear"
	default:
		return "noLimit"
	}
}

func mapBaiduRecencyFilter(rangeCode string) string {
	switch rangeCode {
	case "d", "w":
		// Baidu does not expose a day-level filter. Use the closest supported
		// window to keep recency bias instead of silently dropping the filter.
		return "week"
	case "m":
		return "month"
	case "y":
		return "year"
	default:
		return ""
	}
}

func mapKagiLensTimeFilter(rangeCode string, now time.Time) *kagiopenapi.SearchRequestLens {
	lens := kagiopenapi.NewSearchRequestLens()
	switch rangeCode {
	case "d":
		lens.SetTimeRelative("day")
	case "w":
		lens.SetTimeRelative("week")
	case "m":
		lens.SetTimeRelative("month")
	case "y":
		lens.SetTimeAfter(now.AddDate(-1, 0, 0).Format("2006-01-02"))
	default:
		return nil
	}
	return lens
}

type BraveSearchProvider struct {
	keyPool *APIKeyPool
	proxy   string
	client  *http.Client
}

func (p *BraveSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.keyPool == nil || len(p.keyPool.keys) == 0 {
		return "", errors.New("no API key provided")
	}

	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), count)
	if freshness := mapBraveFreshness(rangeCode); freshness != "" {
		searchURL += "&freshness=" + url.QueryEscape(freshness)
	}

	var lastErr error
	iter := p.keyPool.NewIterator()

	for {
		apiKey, ok := iter.Next()
		if !ok {
			break
		}

		req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", apiKey)

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusTooManyRequests ||
				resp.StatusCode == http.StatusUnauthorized ||
				resp.StatusCode == http.StatusForbidden ||
				resp.StatusCode >= 500 {
				continue
			}
			return "", lastErr
		}

		var searchResp struct {
			Web struct {
				Results []struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Description string `json:"description"`
				} `json:"results"`
			} `json:"web"`
		}

		if err := json.Unmarshal(body, &searchResp); err != nil {
			// Log error body for debugging
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		results := searchResp.Web.Results
		if len(results) == 0 {
			// Log a warning when the API returned 200 but no results.
			// This helps diagnose API format changes or silent errors
			// where the response body does not match the expected structure.
			bodyPreview := string(body)
			if len(bodyPreview) > 300 {
				bodyPreview = bodyPreview[:300]
			}
			logger.WarnCF("web_search", "Brave API returned empty results",
				map[string]any{
					"query":        query,
					"status":       resp.StatusCode,
					"body_preview": bodyPreview,
				})
			return fmt.Sprintf("No results for: %s", query), nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Results for: %s", query))
		for i, item := range results {
			if i >= count {
				break
			}
			lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
			if item.Description != "" {
				lines = append(lines, fmt.Sprintf("   %s", item.Description))
			}
		}

		return strings.Join(lines, "\n"), nil
	}

	return "", fmt.Errorf("all api keys failed, last error: %w", lastErr)
}

type TavilySearchProvider struct {
	keyPool         *APIKeyPool
	baseURL         string
	proxy           string
	client          *http.Client
	searchDepth     string
	chunksPerSource int
	maxContentChars int
}

func (p *TavilySearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.keyPool == nil || len(p.keyPool.keys) == 0 {
		return "", errors.New("no API key provided")
	}

	searchURL := p.baseURL
	if searchURL == "" {
		searchURL = "https://api.tavily.com/search"
	}

	var lastErr error
	iter := p.keyPool.NewIterator()

	for {
		apiKey, ok := iter.Next()
		if !ok {
			break
		}

		depth := p.searchDepth
		if depth == "" {
			depth = config.DefaultTavilySearchDepth
		}
		chunks := p.chunksPerSource
		if chunks <= 0 {
			chunks = config.DefaultTavilyChunksPerSource
		}
		payload := map[string]any{
			"api_key":             apiKey,
			"query":               query,
			"search_depth":        depth,
			"include_answer":      false,
			"include_images":      false,
			"include_raw_content": false,
			"include_usage":       true,
			"max_results":         count,
		}
		if depth != "ultra-fast" {
			payload["chunks_per_source"] = chunks
		}
		if timeRange := mapTavilyTimeRange(rangeCode); timeRange != "" {
			payload["time_range"] = timeRange
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to marshal payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("tavily api error (status %d): %s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusTooManyRequests ||
				resp.StatusCode == http.StatusUnauthorized ||
				resp.StatusCode == http.StatusForbidden ||
				resp.StatusCode >= 500 {
				continue
			}
			return "", lastErr
		}

		var searchResp struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
			Usage struct {
				Credits float64 `json:"credits"`
			} `json:"usage"`
		}

		if err := json.Unmarshal(body, &searchResp); err != nil {
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		results := searchResp.Results
		if len(results) == 0 {
			return fmt.Sprintf("No results for: %s", query), nil
		}

		if searchResp.Usage.Credits > 0 {
			logger.InfoCF("web_search", "Tavily search completed", map[string]any{
				"credits": searchResp.Usage.Credits,
				"depth":   depth,
				"results": len(results),
			})
		}

		maxContent := p.maxContentChars
		if maxContent <= 0 {
			maxContent = config.DefaultTavilyMaxContentChars
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Results for: %s (via Tavily)", query))
		for i, item := range results {
			if i >= count {
				break
			}
			lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
			if content := strings.TrimSpace(item.Content); content != "" {
				lines = append(lines, fmt.Sprintf("   %s", clampText(content, maxContent)))
			}
		}

		return strings.Join(lines, "\n"), nil
	}

	return "", fmt.Errorf("all api keys failed, last error: %w", lastErr)
}

type KagiSearchProvider struct {
	keyPool *APIKeyPool
	baseURL string
	proxy   string
	client  *http.Client
}

func (p *KagiSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.keyPool == nil || len(p.keyPool.keys) == 0 {
		return "", errors.New("no API key provided")
	}

	client := p.client
	if client == nil {
		client = &http.Client{Timeout: searchTimeout}
	}

	apiClient := newKagiAPIClient(client, p.baseURL)
	searchReq := kagiopenapi.NewSearchRequest(query)
	searchReq.SetLimit(int32(count))
	if lens := mapKagiLensTimeFilter(rangeCode, time.Now().UTC()); lens != nil {
		searchReq.SetLens(*lens)
	}

	var lastErr error
	iter := p.keyPool.NewIterator()

	for {
		apiKey, ok := iter.Next()
		if !ok {
			break
		}

		authCtx := context.WithValue(ctx, kagiopenapi.ContextAccessToken, apiKey)
		searchResp, httpResp, err := apiClient.SearchAPI.Search(authCtx).SearchRequest(*searchReq).Execute()
		if httpResp != nil && httpResp.Body != nil {
			defer httpResp.Body.Close()
		}
		if err != nil {
			if httpResp != nil {
				if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
					results, parseErr := fallbackKagiSearchResults(httpResp, count)
					if parseErr != nil {
						return "", parseErr
					}
					return formatKagiSearchResults(query, results), nil
				}
				lastErr = kagiStatusError(httpResp.StatusCode)
				if httpResp.StatusCode == http.StatusTooManyRequests ||
					httpResp.StatusCode == http.StatusUnauthorized ||
					httpResp.StatusCode == http.StatusForbidden ||
					httpResp.StatusCode >= 500 {
					continue
				}
				return "", lastErr
			}
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		results := kagiSearchResults(searchResp, count)
		if len(results) == 0 {
			return fmt.Sprintf("No results for: %s", query), nil
		}

		return formatKagiSearchResults(query, results), nil
	}

	return "", fmt.Errorf("all api keys failed, last error: %w", lastErr)
}

func newKagiAPIClient(client *http.Client, baseURL string) *kagiopenapi.APIClient {
	cfg := kagiopenapi.NewConfiguration()
	cfg.UserAgent = fmt.Sprintf(userAgentHonest, config.Version)
	cfg.HTTPClient = client
	cfg.Servers = kagiopenapi.ServerConfigurations{{
		URL:         kagiServerURL(baseURL),
		Description: "Kagi Search API endpoint",
	}}
	return kagiopenapi.NewAPIClient(cfg)
}

func formatKagiSearchResults(query string, results []SearchResultItem) string {
	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query)
	}
	lines := []string{fmt.Sprintf("Results for: %s (via Kagi)", query)}
	for i, item := range results {
		title := item.Title
		if title == "" {
			title = item.URL
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, title, item.URL))
		if item.Published != "" {
			lines = append(lines, fmt.Sprintf("   Published: %s", item.Published))
		}
		if item.Snippet != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.Snippet))
		}
	}
	return strings.Join(lines, "\n")
}

func kagiServerURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "https://kagi.com/api/v1"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSuffix(baseURL, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/search") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/search")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func kagiStatusError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("Kagi Search API authentication failed (status %d)", statusCode)
	case http.StatusForbidden:
		return fmt.Errorf("Kagi Search API request forbidden (status %d)", statusCode)
	case http.StatusTooManyRequests:
		return fmt.Errorf("Kagi Search API rate limited (status %d)", statusCode)
	default:
		if statusCode >= 500 {
			return fmt.Errorf("Kagi Search API server error (status %d)", statusCode)
		}
		return fmt.Errorf("Kagi Search API error (status %d)", statusCode)
	}
}

type kagiFallbackResult struct {
	Type      int    `json:"t"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Snippet   string `json:"snippet"`
	Time      string `json:"time"`
	Published string `json:"published"`
}

func fallbackKagiSearchResults(resp *http.Response, count int) ([]SearchResultItem, error) {
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("failed to parse response: empty response body")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	return parseFallbackKagiSearchResults(body, count)
}

func parseFallbackKagiSearchResults(body []byte, count int) ([]SearchResultItem, error) {
	if count <= 0 {
		count = 10
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	data := bytes.TrimSpace(envelope.Data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, nil
	}

	results := make([]SearchResultItem, 0, count)
	switch data[0] {
	case '{':
		var modern struct {
			Search []kagiFallbackResult `json:"search"`
		}
		if err := json.Unmarshal(data, &modern); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		appendFallbackKagiResults(&results, modern.Search, count, false)
	case '[':
		var legacy []kagiFallbackResult
		if err := json.Unmarshal(data, &legacy); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		appendFallbackKagiResults(&results, legacy, count, true)
	default:
		return nil, fmt.Errorf("failed to parse response: unexpected data shape")
	}
	return results, nil
}

func appendFallbackKagiResults(
	results *[]SearchResultItem,
	items []kagiFallbackResult,
	count int,
	requireLegacyType bool,
) {
	for _, item := range items {
		if len(*results) >= count {
			return
		}
		if requireLegacyType && item.Type != 0 {
			continue
		}
		urlStr := strings.TrimSpace(item.URL)
		if urlStr == "" {
			continue
		}
		published := strings.TrimSpace(item.Published)
		if published == "" {
			published = strings.TrimSpace(item.Time)
		}
		*results = append(*results, SearchResultItem{
			Title:     cleanSearchText(item.Title),
			URL:       urlStr,
			Snippet:   cleanSearchText(item.Snippet),
			Published: published,
		})
	}
}

func kagiSearchResults(searchResp *kagiopenapi.Search200Response, count int) []SearchResultItem {
	if count <= 0 {
		count = 10
	}
	if searchResp == nil || searchResp.Data == nil {
		return nil
	}

	results := make([]SearchResultItem, 0, count)
	for _, item := range searchResp.Data.Search {
		if len(results) >= count {
			break
		}
		urlStr := strings.TrimSpace(item.GetUrl())
		if urlStr == "" {
			continue
		}
		results = append(results, SearchResultItem{
			Title:     cleanSearchText(item.GetTitle()),
			URL:       urlStr,
			Snippet:   cleanSearchText(item.GetSnippet()),
			Published: strings.TrimSpace(item.GetTime()),
		})
	}
	return results
}

func cleanSearchText(content string) string {
	return strings.TrimSpace(html.UnescapeString(stripTags(content)))
}

type SogouSearchProvider struct {
	proxy  string
	client *http.Client
}

type GeminiSearchProvider struct {
	apiKey string
	model  string
	proxy  string
	client *http.Client
}

func (p *GeminiSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return "", errors.New("no API key provided")
	}
	model := strings.TrimSpace(p.model)
	if model == "" {
		model = "gemini-2.5-flash"
	}

	payload := map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]string{{"text": query}},
		}},
		"tools": []map[string]any{{"google_search": map[string]any{}}},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		url.PathEscape(model),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", p.apiKey)
	req.Header.Set("User-Agent", fmt.Sprintf(userAgentHonest, config.Version))

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini search api error (status %d): %s", resp.StatusCode, string(body))
	}

	var searchResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				GroundingChunks []struct {
					Web struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web"`
				} `json:"groundingChunks"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(searchResp.Candidates) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	candidate := searchResp.Candidates[0]
	lines := []string{fmt.Sprintf("Results for: %s (via Gemini Google Search)", query)}
	for _, part := range candidate.Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			lines = append(lines, strings.TrimSpace(part.Text))
		}
	}
	citationCount := 0
	for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
		if strings.TrimSpace(chunk.Web.URI) == "" {
			continue
		}
		citationCount++
		title := strings.TrimSpace(chunk.Web.Title)
		if title == "" {
			title = chunk.Web.URI
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", citationCount, title, chunk.Web.URI))
		if citationCount >= count {
			break
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (p *SogouSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	const sogouWAPURL = "https://wap.sogou.com/web/searchList.jsp"

	results := make([]SearchResultItem, 0, count)
	seenURLs := make(map[string]bool)
	maxPages := min(3, (count+1)/2+1)

	for page := 1; page <= maxPages && len(results) < count; page++ {
		params := url.Values{}
		params.Set("keyword", applySogouRangeHint(query, rangeCode))
		params.Set("v", "5")
		params.Set("p", fmt.Sprintf("%d", page))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sogouWAPURL+"?"+params.Encode(), nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", sogouUserAgent)

		resp, err := p.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("request failed: %w", err)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Sogou returned status %d", resp.StatusCode)
		}

		html := string(body)
		if len(html) < 200 {
			break
		}

		matches := reSogouTitle.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}

			title := stripTags(match[2])
			link := extractSogouURL(match[1])
			if title == "" || link == "" || seenURLs[link] {
				continue
			}
			seenURLs[link] = true

			start := strings.Index(html, match[0])
			snippet := ""
			if start >= 0 {
				after := html[start+len(match[0]):]
				if len(after) > 2000 {
					after = after[:2000]
				}
				if snippetMatch := reSogouSnippet.FindStringSubmatch(after); len(snippetMatch) > 1 {
					snippet = stripTags(snippetMatch[1])
				}
			}

			results = append(results, SearchResultItem{
				Title:   title,
				URL:     link,
				Snippet: snippet,
			})
			if len(results) >= count {
				break
			}
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	lines := []string{fmt.Sprintf("Results for: %s (via Sogou)", query)}
	for i, item := range results {
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
		if item.Snippet != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.Snippet))
		}
	}
	return strings.Join(lines, "\n"), nil
}

type DuckDuckGoSearchProvider struct {
	proxy    string
	client   *http.Client
	endpoint string
}

var (
	errSearchProviderBlocked = errors.New("search provider blocked")
	errSearchProviderBusy    = errors.New("search provider busy")
)

const (
	ddgDefaultEndpoint = "https://html.duckduckgo.com/html/"
	ddgGateMaxWait     = 2 * time.Second
	ddgMinInterval     = 2 * time.Second
)

var (
	// ddgBlockCooldown is a variable so tests can shorten it.
	ddgBlockCooldown = 60 * time.Second
	ddgState         = &providerBlockState{}
	ddgGate          = &providerGate{minInterval: ddgMinInterval}
)

// providerBlockState keeps one cooldown per provider for the whole process.
// Web search tools are created per agent, so per-instance state would let
// concurrent agents keep hammering a blocked provider.
type providerBlockState struct {
	mu           sync.Mutex
	blockedUntil time.Time
}

func (s *providerBlockState) remaining() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wait := time.Until(s.blockedUntil); wait > 0 {
		return wait
	}
	return 0
}

func (s *providerBlockState) block(d time.Duration) {
	if d <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	until := time.Now().Add(d)
	if until.After(s.blockedUntil) {
		s.blockedUntil = until
	}
}

func (s *providerBlockState) reset() {
	s.mu.Lock()
	s.blockedUntil = time.Time{}
	s.mu.Unlock()
}

// providerGate spaces requests to one endpoint. A caller that would have to
// wait longer than ddgGateMaxWait gets errSearchProviderBusy so the tool can
// fall through to the next provider instead of stalling the turn.
type providerGate struct {
	mu          sync.Mutex
	last        time.Time
	minInterval time.Duration
}

func (g *providerGate) acquire(ctx context.Context) error {
	g.mu.Lock()
	now := time.Now()
	slot := g.last.Add(g.minInterval)
	if slot.Before(now) {
		slot = now
	}
	wait := time.Until(slot)
	if wait > ddgGateMaxWait {
		g.mu.Unlock()
		return errSearchProviderBusy
	}
	g.last = slot
	g.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (g *providerGate) reset() {
	g.mu.Lock()
	g.last = time.Time{}
	g.mu.Unlock()
}

func (p *DuckDuckGoSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if wait := ddgState.remaining(); wait > 0 {
		return "", fmt.Errorf("%w: cooling down for %s", errSearchProviderBlocked, wait.Round(time.Second))
	}
	if err := ddgGate.acquire(ctx); err != nil {
		return "", err
	}

	endpoint := strings.TrimSpace(p.endpoint)
	if endpoint == "" {
		endpoint = ddgDefaultEndpoint
	}
	searchURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid DuckDuckGo endpoint: %w", err)
	}
	params := searchURL.Query()
	params.Set("q", query)
	if dateFilter := mapDuckDuckGoDateFilter(rangeCode); dateFilter != "" {
		params.Set("df", dateFilter)
	}
	searchURL.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if isDDGBlockStatus(resp.StatusCode) {
		ddgState.block(ddgBlockCooldown)
		logger.WarnCF("web_search", "DuckDuckGo blocked; cooling down", map[string]any{
			"status":   resp.StatusCode,
			"cooldown": ddgBlockCooldown.String(),
		})
		return "", fmt.Errorf("%w: status %d", errSearchProviderBlocked, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("duckduckgo status %d", resp.StatusCode)
	}

	return p.extractResults(string(body), count, query)
}

func (p *DuckDuckGoSearchProvider) extractResults(
	body string,
	count int,
	query string,
) (string, error) {
	results := parseDDGResults(body, count)
	if len(results) == 0 {
		if hasDDGChallengeMarker(body) {
			ddgState.block(ddgBlockCooldown)
			logger.WarnCF("web_search", "DuckDuckGo challenge page; cooling down", map[string]any{
				"cooldown": ddgBlockCooldown.String(),
			})
			return "", fmt.Errorf("%w: challenge page", errSearchProviderBlocked)
		}
		return fmt.Sprintf("No results found or extraction failed. Query: %s", query), nil
	}

	lines := make([]string, 0, len(results)*2+1)
	lines = append(lines, fmt.Sprintf("Results for: %s (via DuckDuckGo)", query))
	for i, item := range results {
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.title, item.url))
		if item.snippet != "" {
			lines = append(lines, "   "+item.snippet)
		}
	}
	return strings.Join(lines, "\n"), nil
}

type ddgResult struct {
	title   string
	url     string
	snippet string
}

// parseDDGResults walks the result DOM so every snippet stays attached to its
// own result block. The previous regex paired links and snippets by index,
// which could drift when the page contained ads or differently shaped blocks.
func parseDDGResults(body string, count int) []ddgResult {
	if count <= 0 {
		return nil
	}
	doc, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	var results []ddgResult
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if len(results) >= count {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "a" &&
			(hasClassToken(n, "result__a") || hasClassToken(n, "result-link")) {
			block := closestDDGResultBlock(n)
			if block != nil && hasClassToken(block, "result--ad") {
				return
			}
			target := ddgTargetURL(attrValue(n, "href"))
			title := collapseWhitespace(nodeText(n))
			snippet := ""
			if block != nil {
				snippet = ddgSnippetText(block)
			}
			if target != "" && title != "" {
				results = append(results, ddgResult{title: title, url: target, snippet: snippet})
			}
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return results
}

func closestDDGResultBlock(n *xhtml.Node) *xhtml.Node {
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == xhtml.ElementNode && hasClassToken(parent, "result") {
			return parent
		}
	}
	return nil
}

func ddgSnippetText(block *xhtml.Node) string {
	var snippet string
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if snippet != "" {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data != "a" &&
			(hasClassToken(n, "result__snippet") || hasClassToken(n, "result-snippet")) {
			snippet = collapseWhitespace(nodeText(n))
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "a" &&
			hasClassToken(n, "result__snippet") {
			snippet = collapseWhitespace(nodeText(n))
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(block)
	return snippet
}

func hasClassToken(n *xhtml.Node, token string) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	for _, field := range strings.Fields(attrValue(n, "class")) {
		if strings.EqualFold(field, token) {
			return true
		}
	}
	return false
}

func attrValue(n *xhtml.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func nodeText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.TextNode {
			b.WriteString(node.Data)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ddgTargetURL unwraps DuckDuckGo redirect links (uddg=) and drops the
// tracking tail such as "&rut=..." that used to leak into citations.
func ddgTargetURL(raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if target := parsed.Query().Get("uddg"); target != "" {
		return target
	}
	if parsed.Scheme == "" && strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func isDDGBlockStatus(status int) bool {
	switch status {
	case http.StatusAccepted, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func hasDDGChallengeMarker(body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range []string{
		"anomaly",
		"challenge-form",
		"are you a robot",
		"bots use duckduckgo",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func stripTags(content string) string {
	return reTags.ReplaceAllString(content, "")
}

type PerplexitySearchProvider struct {
	keyPool *APIKeyPool
	proxy   string
	client  *http.Client
}

func (p *PerplexitySearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.keyPool == nil || len(p.keyPool.keys) == 0 {
		return "", errors.New("no API key provided")
	}

	searchURL := "https://api.perplexity.ai/chat/completions"

	var lastErr error
	iter := p.keyPool.NewIterator()

	for {
		apiKey, ok := iter.Next()
		if !ok {
			break
		}

		payload := map[string]any{
			"model": "sonar",
			"messages": []map[string]string{
				{
					"role":    "system",
					"content": "You are a search assistant. Provide concise search results with titles, URLs, and brief descriptions in the following format:\n1. Title\n   URL\n   Description\n\nDo not add extra commentary.",
				},
				{
					"role": "user",
					"content": fmt.Sprintf(
						"Search for: %s. Provide up to %d relevant results.",
						query,
						count,
					),
				},
			},
			"max_tokens": 1000,
		}
		if recencyFilter := mapPerplexityRecencyFilter(rangeCode); recencyFilter != "" {
			payload["search_recency_filter"] = recencyFilter
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(
			ctx,
			"POST",
			searchURL,
			strings.NewReader(string(payloadBytes)),
		)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("User-Agent", userAgent)

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("Perplexity API error: %s", string(body))
			if resp.StatusCode == http.StatusTooManyRequests ||
				resp.StatusCode == http.StatusUnauthorized ||
				resp.StatusCode == http.StatusForbidden ||
				resp.StatusCode >= 500 {
				continue
			}
			return "", lastErr
		}

		var searchResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}

		if err := json.Unmarshal(body, &searchResp); err != nil {
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		if len(searchResp.Choices) == 0 {
			return fmt.Sprintf("No results for: %s", query), nil
		}

		return fmt.Sprintf(
			"Results for: %s (via Perplexity)\n%s",
			query,
			searchResp.Choices[0].Message.Content,
		), nil
	}

	return "", fmt.Errorf("all api keys failed, last error: %w", lastErr)
}

type SearXNGSearchProvider struct {
	baseURL string
	proxy   string
	client  *http.Client
}

func (p *SearXNGSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.baseURL == "" {
		return "", errors.New("no SearXNG URL provided")
	}

	searchURL := fmt.Sprintf("%s/search?q=%s&format=json&categories=general",
		strings.TrimSuffix(p.baseURL, "/"),
		url.QueryEscape(query))
	if timeRange := mapSearXNGTimeRange(rangeCode); timeRange != "" {
		searchURL += "&time_range=" + url.QueryEscape(timeRange)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := p.client
	if client == nil {
		client = &http.Client{Timeout: searchTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SearXNG returned status %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Engine  string  `json:"engine"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	// Limit results to requested count
	if len(result.Results) > count {
		result.Results = result.Results[:count]
	}

	// Format results in standard PicoClaw format
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Results for: %s (via SearXNG)\n", query))
	for i, r := range result.Results {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		b.WriteString(fmt.Sprintf("   %s\n", r.URL))
		if r.Content != "" {
			b.WriteString(fmt.Sprintf("   %s\n", r.Content))
		}
	}

	return b.String(), nil
}

type GLMSearchProvider struct {
	apiKey       string
	baseURL      string
	searchEngine string
	proxy        string
	client       *http.Client
}

func (p *GLMSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.apiKey == "" {
		return "", errors.New("no API key provided")
	}

	searchURL := p.baseURL
	if searchURL == "" {
		searchURL = "https://open.bigmodel.cn/api/paas/v4/web_search"
	}

	payload := map[string]any{
		"search_query":  query,
		"search_engine": p.searchEngine,
		"search_intent": false,
		"count":         count,
		"content_size":  "medium",
	}
	if recencyFilter := mapGLMRecencyFilter(rangeCode); recencyFilter != "" {
		payload["search_recency_filter"] = recencyFilter
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GLM Search API error (status %d): %s", resp.StatusCode, string(body))
	}

	var searchResp struct {
		SearchResult []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Link    string `json:"link"`
		} `json:"search_result"`
	}

	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	results := searchResp.SearchResult
	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Results for: %s (via GLM Search)", query))
	for i, item := range results {
		if i >= count {
			break
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.Link))
		if item.Content != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.Content))
		}
	}

	return strings.Join(lines, "\n"), nil
}

type BaiduSearchProvider struct {
	apiKey  string
	baseURL string
	proxy   string
	client  *http.Client
}

func (p *BaiduSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.apiKey == "" {
		return "", errors.New("no API key provided")
	}

	searchURL := p.baseURL
	if searchURL == "" {
		searchURL = "https://qianfan.baidubce.com/v2/ai_search/web_search"
	}

	payload := map[string]any{
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": query,
			},
		},
		"search_source":        "baidu_search_v2",
		"resource_type_filter": []map[string]any{{"type": "web", "top_k": count}},
	}
	if recencyFilter := mapBaiduRecencyFilter(rangeCode); recencyFilter != "" {
		payload["search_recency_filter"] = recencyFilter
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("baidu search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("baidu search API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		References []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"references"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.References) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	lines := []string{fmt.Sprintf("Results for: %s (via Baidu Search)", query)}
	for i, item := range result.References {
		if i >= count {
			break
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
		if item.Content != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.Content))
		}
	}

	return strings.Join(lines, "\n"), nil
}

type WebSearchTool struct {
	provider          SearchProvider
	maxResults        int
	maxOutputChars    int
	providerResolver  func(query string) (SearchProvider, int)
	candidateResolver func(query string) []searchCandidate
}

type searchCandidate struct {
	name       string
	provider   SearchProvider
	maxResults int
}

type WebSearchToolOptions struct {
	Provider              string
	BraveAPIKeys          []string
	BraveMaxResults       int
	BraveEnabled          bool
	TavilyAPIKeys         []string
	TavilyBaseURL         string
	TavilyMaxResults      int
	TavilyEnabled         bool
	TavilySearchDepth     string
	TavilyChunksPerSource int
	TavilyMaxContentChars int
	SearchMaxChars        int
	KagiAPIKeys           []string
	KagiBaseURL           string
	KagiMaxResults        int
	KagiEnabled           bool
	SogouMaxResults       int
	SogouEnabled          bool
	DuckDuckGoMaxResults  int
	DuckDuckGoEnabled     bool
	GeminiAPIKey          string
	GeminiModel           string
	GeminiMaxResults      int
	GeminiEnabled         bool
	PerplexityAPIKeys     []string
	PerplexityMaxResults  int
	PerplexityEnabled     bool
	SearXNGBaseURL        string
	SearXNGMaxResults     int
	SearXNGEnabled        bool
	GLMSearchAPIKey       string
	GLMSearchBaseURL      string
	GLMSearchEngine       string
	GLMSearchMaxResults   int
	GLMSearchEnabled      bool
	BaiduSearchAPIKey     string
	BaiduSearchBaseURL    string
	BaiduSearchMaxResults int
	BaiduSearchEnabled    bool
	Proxy                 string
}

func WebSearchToolOptionsFromConfig(cfg *config.Config) WebSearchToolOptions {
	return WebSearchToolOptions{
		Provider:              cfg.Tools.Web.Provider,
		BraveAPIKeys:          cfg.Tools.Web.Brave.APIKeys.Values(),
		BraveMaxResults:       cfg.Tools.Web.Brave.MaxResults,
		BraveEnabled:          cfg.Tools.Web.Brave.Enabled,
		TavilyAPIKeys:         cfg.Tools.Web.Tavily.APIKeys.Values(),
		TavilyBaseURL:         cfg.Tools.Web.Tavily.BaseURL,
		TavilyMaxResults:      cfg.Tools.Web.Tavily.MaxResults,
		TavilyEnabled:         cfg.Tools.Web.Tavily.Enabled,
		TavilySearchDepth:     cfg.Tools.Web.Tavily.GetSearchDepth(),
		TavilyChunksPerSource: cfg.Tools.Web.Tavily.GetChunksPerSource(),
		TavilyMaxContentChars: cfg.Tools.Web.Tavily.GetMaxContentChars(),
		SearchMaxChars:        cfg.Tools.Web.GetSearchMaxChars(),
		KagiAPIKeys:           cfg.Tools.Web.Kagi.APIKeys.Values(),
		KagiBaseURL:           cfg.Tools.Web.Kagi.BaseURL,
		KagiMaxResults:        cfg.Tools.Web.Kagi.MaxResults,
		KagiEnabled:           cfg.Tools.Web.Kagi.Enabled,
		SogouMaxResults:       cfg.Tools.Web.Sogou.MaxResults,
		SogouEnabled:          cfg.Tools.Web.Sogou.Enabled,
		DuckDuckGoMaxResults:  cfg.Tools.Web.DuckDuckGo.MaxResults,
		DuckDuckGoEnabled:     cfg.Tools.Web.DuckDuckGo.Enabled,
		GeminiAPIKey:          cfg.Tools.Web.Gemini.APIKey.String(),
		GeminiModel:           cfg.Tools.Web.Gemini.Model,
		GeminiMaxResults:      cfg.Tools.Web.Gemini.MaxResults,
		GeminiEnabled:         cfg.Tools.Web.Gemini.Enabled,
		PerplexityAPIKeys:     cfg.Tools.Web.Perplexity.APIKeys.Values(),
		PerplexityMaxResults:  cfg.Tools.Web.Perplexity.MaxResults,
		PerplexityEnabled:     cfg.Tools.Web.Perplexity.Enabled,
		SearXNGBaseURL:        cfg.Tools.Web.SearXNG.BaseURL,
		SearXNGMaxResults:     cfg.Tools.Web.SearXNG.MaxResults,
		SearXNGEnabled:        cfg.Tools.Web.SearXNG.Enabled,
		GLMSearchAPIKey:       cfg.Tools.Web.GLMSearch.APIKey.String(),
		GLMSearchBaseURL:      cfg.Tools.Web.GLMSearch.BaseURL,
		GLMSearchEngine:       cfg.Tools.Web.GLMSearch.SearchEngine,
		GLMSearchMaxResults:   cfg.Tools.Web.GLMSearch.MaxResults,
		GLMSearchEnabled:      cfg.Tools.Web.GLMSearch.Enabled,
		BaiduSearchAPIKey:     cfg.Tools.Web.BaiduSearch.APIKey.String(),
		BaiduSearchBaseURL:    cfg.Tools.Web.BaiduSearch.BaseURL,
		BaiduSearchMaxResults: cfg.Tools.Web.BaiduSearch.MaxResults,
		BaiduSearchEnabled:    cfg.Tools.Web.BaiduSearch.Enabled,
		Proxy:                 cfg.Tools.Web.Proxy,
	}
}

func WebSearchProviderReady(opts WebSearchToolOptions, name string) bool {
	return opts.providerReady(name)
}

func ResolveWebSearchProviderName(opts WebSearchToolOptions, query string) (string, error) {
	return opts.resolveProviderName(query)
}

var (
	knownWebSearchProviders = []string{
		"sogou",
		"duckduckgo",
		"gemini",
		"brave",
		"tavily",
		"kagi",
		"perplexity",
		"searxng",
		"glm_search",
		"baidu_search",
	}
	autoFreeWebSearchProviders = []string{"duckduckgo", "searxng", "sogou"}
	autoPaidWebSearchProviders = []string{
		"tavily", "brave", "kagi", "perplexity", "gemini", "baidu_search", "glm_search",
	}
)

func isKnownWebSearchProvider(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, known := range knownWebSearchProviders {
		if name == known {
			return true
		}
	}
	return false
}

func (opts WebSearchToolOptions) providerReady(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sogou":
		return opts.SogouEnabled
	case "duckduckgo":
		return opts.DuckDuckGoEnabled
	case "gemini":
		return opts.GeminiEnabled && strings.TrimSpace(opts.GeminiAPIKey) != ""
	case "brave":
		return opts.BraveEnabled && len(opts.BraveAPIKeys) > 0
	case "tavily":
		return opts.TavilyEnabled && len(opts.TavilyAPIKeys) > 0
	case "kagi":
		return opts.KagiEnabled && len(opts.KagiAPIKeys) > 0
	case "perplexity":
		return opts.PerplexityEnabled && len(opts.PerplexityAPIKeys) > 0
	case "searxng":
		return opts.SearXNGEnabled && strings.TrimSpace(opts.SearXNGBaseURL) != ""
	case "glm_search":
		return opts.GLMSearchEnabled && strings.TrimSpace(opts.GLMSearchAPIKey) != ""
	case "baidu_search":
		return opts.BaiduSearchEnabled && strings.TrimSpace(opts.BaiduSearchAPIKey) != ""
	default:
		return false
	}
}

func (opts WebSearchToolOptions) normalizedProviderName() string {
	providerName := strings.ToLower(strings.TrimSpace(opts.Provider))
	if providerName != "" && providerName != "auto" && !isKnownWebSearchProvider(providerName) {
		// Tolerate stale or manually edited config values at runtime by
		// treating them as "auto" and falling back to the next ready provider.
		return "auto"
	}
	return providerName
}

func (opts WebSearchToolOptions) providerCandidates(query string) []string {
	providerName := opts.normalizedProviderName()
	if providerName != "" && providerName != "auto" && opts.providerReady(providerName) {
		// An explicit provider is strict: no fallback.
		return []string{providerName}
	}

	candidates := make([]string, 0, len(knownWebSearchProviders))
	for _, name := range autoFreeWebSearchProviders {
		if opts.providerReady(name) {
			candidates = append(candidates, name)
		}
	}
	for _, name := range autoPaidWebSearchProviders {
		if opts.providerReady(name) {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func (opts WebSearchToolOptions) resolveProviderName(query string) (string, error) {
	candidates := opts.providerCandidates(query)
	if len(candidates) == 0 {
		return "", nil
	}
	return candidates[0], nil
}

func (opts WebSearchToolOptions) providerByName(name string) (SearchProvider, int, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return nil, 0, nil
	case "sogou":
		if !opts.providerReady("sogou") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Sogou: %w", err)
		}
		maxResults := 10
		if opts.SogouMaxResults > 0 {
			maxResults = min(opts.SogouMaxResults, 10)
		}
		return &SogouSearchProvider{
			proxy:  opts.Proxy,
			client: client,
		}, maxResults, nil
	case "perplexity":
		if !opts.providerReady("perplexity") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, perplexityTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Perplexity: %w", err)
		}
		maxResults := 10
		if opts.PerplexityMaxResults > 0 {
			maxResults = min(opts.PerplexityMaxResults, 10)
		}
		return &PerplexitySearchProvider{
			keyPool: NewAPIKeyPool(opts.PerplexityAPIKeys),
			proxy:   opts.Proxy,
			client:  client,
		}, maxResults, nil
	case "brave":
		if !opts.providerReady("brave") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Brave: %w", err)
		}
		maxResults := 10
		if opts.BraveMaxResults > 0 {
			maxResults = min(opts.BraveMaxResults, 10)
		}
		return &BraveSearchProvider{
			keyPool: NewAPIKeyPool(opts.BraveAPIKeys),
			proxy:   opts.Proxy,
			client:  client,
		}, maxResults, nil
	case "gemini":
		if !opts.providerReady("gemini") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Gemini: %w", err)
		}
		maxResults := 10
		if opts.GeminiMaxResults > 0 {
			maxResults = min(opts.GeminiMaxResults, 10)
		}
		return &GeminiSearchProvider{
			apiKey: opts.GeminiAPIKey,
			model:  opts.GeminiModel,
			proxy:  opts.Proxy,
			client: client,
		}, maxResults, nil
	case "searxng":
		if !opts.providerReady("searxng") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for SearXNG: %w", err)
		}
		maxResults := 10
		if opts.SearXNGMaxResults > 0 {
			maxResults = min(opts.SearXNGMaxResults, 10)
		}
		return &SearXNGSearchProvider{
			baseURL: opts.SearXNGBaseURL,
			proxy:   opts.Proxy,
			client:  client,
		}, maxResults, nil
	case "tavily":
		if !opts.providerReady("tavily") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Tavily: %w", err)
		}
		maxResults := 10
		if opts.TavilyMaxResults > 0 {
			maxResults = min(opts.TavilyMaxResults, 10)
		}
		return &TavilySearchProvider{
			keyPool:         NewAPIKeyPool(opts.TavilyAPIKeys),
			baseURL:         opts.TavilyBaseURL,
			proxy:           opts.Proxy,
			client:          client,
			searchDepth:     opts.TavilySearchDepth,
			chunksPerSource: opts.TavilyChunksPerSource,
			maxContentChars: opts.TavilyMaxContentChars,
		}, maxResults, nil
	case "kagi":
		if !opts.providerReady("kagi") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Kagi: %w", err)
		}
		maxResults := 10
		if opts.KagiMaxResults > 0 {
			maxResults = min(opts.KagiMaxResults, 10)
		}
		return &KagiSearchProvider{
			keyPool: NewAPIKeyPool(opts.KagiAPIKeys),
			baseURL: opts.KagiBaseURL,
			proxy:   opts.Proxy,
			client:  client,
		}, maxResults, nil
	case "duckduckgo":
		if !opts.providerReady("duckduckgo") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for DuckDuckGo: %w", err)
		}
		maxResults := 10
		if opts.DuckDuckGoMaxResults > 0 {
			maxResults = min(opts.DuckDuckGoMaxResults, 10)
		}
		return &DuckDuckGoSearchProvider{
			proxy:  opts.Proxy,
			client: client,
		}, maxResults, nil
	case "baidu_search":
		if !opts.providerReady("baidu_search") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, perplexityTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Baidu Search: %w", err)
		}
		maxResults := 10
		if opts.BaiduSearchMaxResults > 0 {
			maxResults = min(opts.BaiduSearchMaxResults, 10)
		}
		return &BaiduSearchProvider{
			apiKey:  opts.BaiduSearchAPIKey,
			baseURL: opts.BaiduSearchBaseURL,
			proxy:   opts.Proxy,
			client:  client,
		}, maxResults, nil
	case "glm_search":
		if !opts.providerReady("glm_search") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for GLM Search: %w", err)
		}
		searchEngine := opts.GLMSearchEngine
		if searchEngine == "" {
			searchEngine = "search_std"
		}
		maxResults := 10
		if opts.GLMSearchMaxResults > 0 {
			maxResults = min(opts.GLMSearchMaxResults, 10)
		}
		return &GLMSearchProvider{
			apiKey:       opts.GLMSearchAPIKey,
			baseURL:      opts.GLMSearchBaseURL,
			searchEngine: searchEngine,
			proxy:        opts.Proxy,
			client:       client,
		}, maxResults, nil
	default:
		return nil, 0, fmt.Errorf("unknown web search provider %q", name)
	}
}

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func containsLatinLetter(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) && unicode.In(r, unicode.Latin) {
			return true
		}
	}
	return false
}

func prefersDuckDuckGoQuery(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if containsHan(trimmed) {
		return false
	}
	if containsLatinLetter(trimmed) {
		return true
	}
	return false
}

func (opts WebSearchToolOptions) buildCandidateResolver() (func(query string) []searchCandidate, error) {
	providersByName := make(map[string]SearchProvider, len(knownWebSearchProviders))
	maxResultsByName := make(map[string]int, len(knownWebSearchProviders))

	for _, name := range knownWebSearchProviders {
		if !opts.providerReady(name) {
			continue
		}
		provider, maxResults, err := opts.providerByName(name)
		if err != nil {
			return nil, err
		}
		if provider == nil {
			continue
		}
		providersByName[name] = provider
		maxResultsByName[name] = maxResults
	}

	return func(query string) []searchCandidate {
		names := opts.providerCandidates(query)
		candidates := make([]searchCandidate, 0, len(names))
		for _, name := range names {
			provider, ok := providersByName[name]
			if !ok {
				continue
			}
			candidates = append(candidates, searchCandidate{
				name:       name,
				provider:   provider,
				maxResults: maxResultsByName[name],
			})
		}
		return candidates
	}, nil
}

// buildProviderResolver keeps the single-provider resolver used by tests and
// older construction paths. New tools use buildCandidateResolver so a failed
// provider can fall through to the next one.
func (opts WebSearchToolOptions) buildProviderResolver() (func(query string) (SearchProvider, int), error) {
	resolver, err := opts.buildCandidateResolver()
	if err != nil {
		return nil, err
	}
	return func(query string) (SearchProvider, int) {
		candidates := resolver(query)
		if len(candidates) == 0 {
			return nil, 0
		}
		return candidates[0].provider, candidates[0].maxResults
	}, nil
}

func searchOutputEmpty(output string) bool {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return true
	}
	return strings.HasPrefix(trimmed, "No results for:") ||
		strings.HasPrefix(trimmed, "No results found or extraction failed.")
}

// truncateSearchOutput caps a provider response so a chatty provider cannot
// fill the model's context with one call. The cut prefers a line boundary and
// marks the truncation explicitly.
func truncateSearchOutput(output string, limit int) string {
	if limit <= 0 {
		return output
	}
	runes := []rune(output)
	if len(runes) <= limit {
		return output
	}
	cut := limit
	for i := limit - 1; i > limit/2; i-- {
		if runes[i] == '\n' {
			cut = i
			break
		}
	}
	trimmed := strings.TrimRight(string(runes[:cut]), "\n")
	return trimmed + "\n... (truncated)"
}

// clampText shortens s to at most limit runes and marks the cut.
func clampText(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimRight(string(runes[:limit]), " ") + "…"
}

func NewWebSearchTool(opts WebSearchToolOptions) (*WebSearchTool, error) {
	resolver, err := opts.buildCandidateResolver()
	if err != nil {
		return nil, err
	}
	candidates := resolver("")
	if len(candidates) == 0 {
		return nil, nil
	}

	return &WebSearchTool{
		provider:          candidates[0].provider,
		maxResults:        candidates[0].maxResults,
		maxOutputChars:    opts.SearchMaxChars,
		candidateResolver: resolver,
	}, nil
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Search the web for current information. Supports query, count, and an optional temporal range filter. Returns titles, URLs, and snippets from search results."
}

func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "Number of results (default: 10, max: 10)",
				"minimum":     1.0,
				"maximum":     10.0,
			},
			"range": map[string]any{
				"type":        "string",
				"description": "Optional time filter: d (day), w (week), m (month), y (year)",
				"enum":        []string{"d", "w", "m", "y"},
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ErrorResult("query is required")
	}
	query = strings.TrimSpace(query)

	provider := t.provider
	maxResults := t.maxResults
	if t.providerResolver != nil {
		provider, maxResults = t.providerResolver(query)
	}
	candidates := []searchCandidate(nil)
	if t.candidateResolver != nil {
		candidates = t.candidateResolver(query)
	} else if provider != nil {
		candidates = []searchCandidate{{
			name:       "provider",
			provider:   provider,
			maxResults: maxResults,
		}}
	}
	if len(candidates) == 0 {
		return ErrorResult("search provider is not configured")
	}

	count64, err := getInt64Arg(args, "count", int64(candidates[0].maxResults))
	if err != nil {
		return ErrorResult(err.Error())
	}

	rangeCode, err := normalizeSearchRange("")
	if err != nil {
		return ErrorResult(err.Error())
	}
	if rawRange, exists := args["range"]; exists {
		rangeStr, ok := rawRange.(string)
		if !ok {
			return ErrorResult("range must be a string")
		}
		rangeCode, err = normalizeSearchRange(rangeStr)
		if err != nil {
			return ErrorResult(err.Error())
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		count := candidate.maxResults
		if count64 > 0 && count64 <= 10 {
			count = min(int(count64), candidate.maxResults)
		}
		result, searchErr := candidate.provider.Search(ctx, query, count, rangeCode)
		if searchErr != nil {
			lastErr = searchErr
			logger.WarnCF("web_search", "Provider failed; trying the next one", map[string]any{
				"provider": candidate.name,
				"error":    searchErr.Error(),
			})
			continue
		}
		if searchOutputEmpty(result) {
			lastErr = fmt.Errorf("%s returned no results", candidate.name)
			logger.WarnCF("web_search", "Provider returned no results; trying the next one",
				map[string]any{"provider": candidate.name})
			continue
		}
		return &ToolResult{
			ForLLM:  truncateSearchOutput(result, t.maxOutputChars),
			ForUser: truncateSearchOutput(result, t.maxOutputChars),
		}
	}

	if lastErr != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", lastErr))
	}
	return ErrorResult("search returned no results")
}

type WebFetchTool struct {
	maxChars        int
	proxy           string
	client          *http.Client
	format          string
	fetchLimitBytes int64
	whitelist       *utils.PrivateHostWhitelist
}

func NewWebFetchTool(maxChars int, format string, fetchLimitBytes int64) (*WebFetchTool, error) {
	// createHTTPClient cannot fail with an empty proxy string.
	return NewWebFetchToolWithConfig(maxChars, "", format, fetchLimitBytes, nil)
}

// allowPrivateWebFetchHosts controls whether loopback/private hosts are allowed.
// This is false in normal runtime to reduce SSRF exposure, and tests can override it temporarily.
var allowPrivateWebFetchHosts atomic.Bool

func NewWebFetchToolWithProxy(
	maxChars int,
	proxy string,
	format string,
	fetchLimitBytes int64,
	privateHostWhitelist []string,
) (*WebFetchTool, error) {
	return NewWebFetchToolWithConfig(maxChars, proxy, format, fetchLimitBytes, privateHostWhitelist)
}

func NewWebFetchToolWithConfig(
	maxChars int,
	proxy string,
	format string,
	fetchLimitBytes int64,
	privateHostWhitelist []string,
) (*WebFetchTool, error) {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	whitelist, err := utils.NewPrivateHostWhitelist(privateHostWhitelist)
	if err != nil {
		return nil, fmt.Errorf("failed to parse web fetch private host whitelist: %w", err)
	}
	client, err := utils.CreateSafeHTTPClient(utils.SafeHTTPClientOptions{
		ProxyURL:             proxy,
		Timeout:              fetchTimeout,
		PrivateHostWhitelist: privateHostWhitelist,
		AllowPrivateHosts: func() bool {
			return allowPrivateWebFetchHosts.Load()
		},
		MaxRedirects: maxRedirects,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client for web fetch: %w", err)
	}
	// Cloudflare and similar WAFs score HTTP/1.1 bot-like requests lower than
	// HTTP/2 browser-like traffic. A custom http.Transport disables HTTP/2 by
	// default, so enable it explicitly for web page fetches only.
	if transport, ok := client.Transport.(*http.Transport); ok {
		transport.ForceAttemptHTTP2 = true
	}
	if fetchLimitBytes <= 0 {
		fetchLimitBytes = 10 * 1024 * 1024 // Security Fallback
	}
	return &WebFetchTool{
		maxChars:        maxChars,
		proxy:           proxy,
		client:          client,
		format:          format,
		fetchLimitBytes: fetchLimitBytes,
		whitelist:       whitelist,
	}, nil
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch a URL and extract the main readable content (HTML to text), skipping menus, headers, footers and hidden nodes. Use this for articles and documentation. The response is capped; when it is truncated, use offset with the returned next_offset to read more."
}

func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to fetch",
			},
			"maxChars": map[string]any{
				"type":        "integer",
				"description": "Maximum characters to extract",
				"minimum":     100.0,
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Character offset to continue reading; pass next_offset from a previous truncated response",
				"minimum":     0.0,
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	urlStr, ok := args["url"].(string)
	if !ok {
		return ErrorResult("url is required")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid URL: %v", err))
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return ErrorResult("only http/https URLs are allowed")
	}

	if parsedURL.Host == "" {
		return ErrorResult("missing domain in URL")
	}

	// Lightweight pre-flight: block obvious localhost/literal-IP without DNS resolution.
	// The real SSRF guard is newSafeDialContext at connect time.
	hostname := parsedURL.Hostname()
	if utils.IsObviousPrivateHost(hostname, t.whitelist, func() bool {
		return allowPrivateWebFetchHosts.Load()
	}) {
		return ErrorResult("fetching private or local network hosts is not allowed")
	}

	maxChars := t.maxChars
	if mc, ok := args["maxChars"].(float64); ok {
		// The tool's maxChars is a hard ceiling; a smaller request is honored,
		// a larger one is clamped so one page cannot dominate the prompt.
		if requested := int(mc); requested > 100 && requested < maxChars {
			maxChars = requested
		}
	}

	doFetch := func(ua string) (*http.Response, []byte, error) {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if reqErr != nil {
			return nil, nil, fmt.Errorf("failed to create request: %w", reqErr)
		}
		utils.AllowConfiguredProxyFirstHop(req, t.client.Transport)
		req.Header.Set("User-Agent", ua)
		// Browser navigation headers. Accept-Encoding is deliberately left to
		// Go's transport: setting it manually disables transparent gzip
		// decompression and makes Cloudflare challenges more likely.
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")
		resp, doErr := t.client.Do(req)
		if doErr != nil {
			return nil, nil, fmt.Errorf("request failed: %w", doErr)
		}
		resp.Body = http.MaxBytesReader(nil, resp.Body, t.fetchLimitBytes)

		b, readErr := io.ReadAll(resp.Body)
		return resp, b, readErr
	}

	resp, body, err := doFetch(userAgent)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrorResult(
				fmt.Sprintf(
					"failed to read response: size exceeded %d bytes limit",
					t.fetchLimitBytes,
				),
			)
		}
		return ErrorResult(err.Error())
	}

	// Cloudflare (and similar WAFs) signal bot challenges with 403 + cf-mitigated: challenge.
	// Retry once with an honest User-Agent that identifies picoclaw, which some
	// operators explicitly allow-list for AI assistants.
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("Cf-Mitigated") == "challenge" {
		logger.DebugCF("tool", "Cloudflare challenge detected, retrying with honest User-Agent",
			map[string]any{"url": urlStr})
		honestUA := fmt.Sprintf(userAgentHonest, config.Version)
		resp2, body2, err2 := doFetch(honestUA)
		if resp2 != nil && resp2.Body != nil {
			defer resp2.Body.Close()
		}

		if err2 == nil {
			resp, body = resp2, body2
		} else {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err2, &maxBytesErr) {
				return ErrorResult(
					fmt.Sprintf("failed to read response: size exceeded %d bytes limit", t.fetchLimitBytes),
				)
			}
			return ErrorResult(err2.Error())
		}
	}

	bodyStr := string(body)
	contentType := resp.Header.Get("Content-Type")

	// A challenge page is not usable content. Return an explicit error instead
	// of a "successful" fetch with an empty or 57-character body.
	if reason := siteProtectionBlockReason(
		resp.StatusCode,
		resp.Header.Get("Cf-Mitigated"),
		body,
	); reason != "" {
		label := ""
		if reason == "challenge" {
			label = ", Cloudflare challenge"
		}
		return ErrorResult(fmt.Sprintf(
			"Site protection blocked the request (HTTP %d%s). "+
				"The page requires browser verification and returned no usable content; try another source.",
			resp.StatusCode, label,
		))
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// The most common error here is "mime: no media type" if the header is empty.
		logger.WarnCF("tool", "Failed to parse Content-Type", map[string]any{
			"raw_header": contentType,
			"error":      err.Error(),
		})

		// security fallback
		mediaType = "application/octet-stream"
	}

	charset, hasCharset := params["charset"]
	if hasCharset {
		// If the charset is not utf-8, we might have to convert the bodyStr
		// before passing it to the HTML/Markdown parser
		if strings.ToLower(charset) != "utf-8" {
			logger.WarnCF(
				"tool",
				"Note: the content is not in UTF-8",
				map[string]any{"charset": charset},
			)
		}
	}

	var text, extractor string

	switch {
	case mediaType == "application/json":
		var jsonData any
		if err := json.Unmarshal(body, &jsonData); err != nil {
			text = bodyStr
			extractor = "raw"
			break
		}

		formatted, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			text = bodyStr
			extractor = "raw"
			break
		}

		text = string(formatted)
		extractor = "json"

	case mediaType == "text/html" || looksLikeHTML(bodyStr):
		readable := extractReadableContent(bodyStr)
		switch strings.ToLower(t.format) {
		case "markdown":
			if readable.ok && strings.TrimSpace(readable.html) != "" {
				if md, err := utils.HtmlToMarkdown(readable.html); err == nil && strings.TrimSpace(md) != "" {
					text = prependTitle(readable.title, md)
					extractor = readable.source
					break
				}
			}
			var err error
			text, err = utils.HtmlToMarkdown(bodyStr)
			if err != nil {
				return ErrorResult(fmt.Sprintf("failed to HTML to markdown: %v", err))
			}
			extractor = "markdown"

		default:
			if readable.ok {
				text = prependTitle(readable.title, readable.text)
				extractor = readable.source
			} else {
				text = t.extractText(bodyStr)
				extractor = "text"
			}
		}

	default:
		text = bodyStr
		extractor = "raw"
	}

	totalChars := utf8.RuneCountInString(text)
	offset := 0
	if raw, ok := args["offset"]; ok {
		switch value := raw.(type) {
		case float64:
			if value > 0 {
				offset = int(value)
			}
		case int:
			if value > 0 {
				offset = value
			}
		}
	}
	if offset > totalChars {
		offset = totalChars
	}

	runes := []rune(text)
	end := len(runes)
	truncated := false
	if maxChars > 0 && offset+maxChars < len(runes) {
		end = offset + maxChars
		truncated = true
	}
	text = string(runes[offset:end])
	if truncated {
		text += "\n[Content truncated due to size limit]"
	}

	result := map[string]any{
		"url":         urlStr,
		"status":      resp.StatusCode,
		"extractor":   extractor,
		"truncated":   truncated,
		"length":      len(text),
		"total_chars": totalChars,
		"offset":      offset,
		"text":        text,
	}
	if truncated {
		result["next_offset"] = end
	}

	resultJSON, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal result: %v", marshalErr))
	}

	userMessage := fmt.Sprintf(
		"Fetched %d bytes from %s (extractor: %s, truncated: %v)",
		len(text),
		urlStr,
		extractor,
		truncated,
	)
	if truncated {
		userMessage += fmt.Sprintf(", next offset: %d", end)
	}

	return &ToolResult{
		ForLLM:  string(resultJSON),
		ForUser: userMessage,
	}
}

func looksLikeHTML(body string) bool {
	if body == "" {
		return false
	}

	lower := strings.ToLower(body)

	return strings.HasPrefix(body, "<!doctype") ||
		strings.HasPrefix(lower, "<html")
}

// siteProtectionBlockReason reports whether a response is a bot challenge or a
// refusal, instead of real page content. It stays conservative for 2xx
// responses so a normal page that merely contains a challenge phrase is not
// misclassified.
func siteProtectionBlockReason(status int, cfMitigated string, body []byte) string {
	cf := strings.ToLower(strings.TrimSpace(cfMitigated))
	if cf != "" {
		return "challenge"
	}

	lower := strings.ToLower(string(body))
	markers := []string{
		"just a moment...",
		"enable javascript and cookies",
		"verifying you are human",
		"checking your browser",
		"bir dakika lütfen",
		"güvenlik doğrulaması",
		"cf-challenge",
		"__cf_chl",
		"cf_chl_opt",
	}
	hasMarker := false
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			hasMarker = true
			break
		}
	}

	if status >= 400 {
		if hasMarker {
			return "challenge"
		}
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden,
			http.StatusTooManyRequests, http.StatusServiceUnavailable:
			return "refused"
		}
		return ""
	}

	// Some WAFs serve a challenge with HTTP 200. Only treat compact pages with
	// known challenge markers that way.
	if status >= 200 && status < 300 && hasMarker && len(body) < 8*1024 {
		return "challenge"
	}
	return ""
}

func (t *WebFetchTool) extractText(htmlContent string) string {
	return legacyExtractText(htmlContent)
}

func newSafeDialContext(
	dialer *net.Dialer,
	whitelist *utils.PrivateHostWhitelist,
) func(context.Context, string, string) (net.Conn, error) {
	return utils.NewSafeDialContext(dialer, whitelist, func() bool {
		return allowPrivateWebFetchHosts.Load()
	})
}

func newPrivateHostWhitelist(entries []string) (*utils.PrivateHostWhitelist, error) {
	return utils.NewPrivateHostWhitelist(entries)
}

func isPrivateOrRestrictedIP(ip net.IP) bool {
	return utils.IsPrivateOrRestrictedIP(ip)
}
