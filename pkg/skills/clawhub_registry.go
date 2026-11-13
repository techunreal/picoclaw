package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/utils"
)

const (
	defaultClawHubTimeout  = 30 * time.Second
	defaultMaxZipSize      = 50 * 1024 * 1024 // 50 MB
	defaultMaxResponseSize = 2 * 1024 * 1024  // 2 MB
)

func init() {
	RegisterRegistryProviderBuilder("clawhub", func(_ string, cfg config.SkillRegistryConfig) RegistryProvider {
		privateCfg := clawHubRegistryPrivateConfig{}
		if err := cfg.DecodeParam(&privateCfg); err != nil {
			slog.Warn("invalid clawhub private config", "error", err)
		}
		return ClawHubConfig{
			Enabled:         cfg.Enabled,
			BaseURL:         cfg.BaseURL,
			AuthToken:       cfg.AuthToken.String(),
			SearchPath:      privateCfg.SearchPath,
			SkillsPath:      privateCfg.SkillsPath,
			DownloadPath:    privateCfg.DownloadPath,
			Timeout:         privateCfg.Timeout,
			MaxZipSize:      privateCfg.MaxZipSize,
			MaxResponseSize: privateCfg.MaxResponseSize,
		}
	})
}

type clawHubRegistryPrivateConfig struct {
	SearchPath      string `json:"search_path"`
	SkillsPath      string `json:"skills_path"`
	DownloadPath    string `json:"download_path"`
	Timeout         int    `json:"timeout"`
	MaxZipSize      int    `json:"max_zip_size"`
	MaxResponseSize int    `json:"max_response_size"`
}

// ClawHubRegistry implements SkillRegistry for the ClawHub platform.
type ClawHubRegistry struct {
	baseURL         string
	authToken       string // Optional - for elevated rate limits
	searchPath      string // Search API
	skillsPath      string // For retrieving skill metadata
	downloadPath    string // For fetching ZIP files for download
	maxZipSize      int
	maxResponseSize int
	client          *http.Client
}

// NewClawHubRegistry creates a new ClawHub registry client from config.
func NewClawHubRegistry(cfg ClawHubConfig) *ClawHubRegistry {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://clawhub.ai"
	}
	searchPath := cfg.SearchPath
	if searchPath == "" {
		searchPath = "/api/v1/search"
	}
	skillsPath := cfg.SkillsPath
	if skillsPath == "" {
		skillsPath = "/api/v1/skills"
	}
	downloadPath := cfg.DownloadPath
	if downloadPath == "" {
		downloadPath = "/api/v1/download"
	}

	timeout := defaultClawHubTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}

	maxZip := defaultMaxZipSize
	if cfg.MaxZipSize > 0 {
		maxZip = cfg.MaxZipSize
	}

	maxResp := defaultMaxResponseSize
	if cfg.MaxResponseSize > 0 {
		maxResp = cfg.MaxResponseSize
	}

	return &ClawHubRegistry{
		baseURL:         baseURL,
		authToken:       cfg.AuthToken,
		searchPath:      searchPath,
		skillsPath:      skillsPath,
		downloadPath:    downloadPath,
		maxZipSize:      maxZip,
		maxResponseSize: maxResp,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        5,
				IdleConnTimeout:     30 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (c *ClawHubRegistry) Name() string {
	return "clawhub"
}

func (c *ClawHubRegistry) ResolveInstallDirName(target string) (string, error) {
	if err := utils.ValidateSkillIdentifier(target); err != nil {
		return "", err
	}
	return target, nil
}

func (c *ClawHubRegistry) SkillURL(slug, version string) string {
	return c.SkillURLForOwner(slug, "", version)
}

// SkillURLForOwner returns the canonical ClawHub URL. ClawHub canonical URLs
// are scoped by publisher: /{owner}/skills/{slug}.
func (c *ClawHubRegistry) SkillURLForOwner(slug, ownerHandle, _ string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	ownerHandle = normalizeOwnerHandle(ownerHandle)
	if ownerHandle == "" {
		return c.baseURL + "/skills/" + url.PathEscape(slug)
	}
	return c.baseURL + "/" + url.PathEscape(ownerHandle) + "/skills/" + url.PathEscape(slug)
}

func normalizeOwnerHandle(owner string) string {
	owner = strings.TrimSpace(owner)
	owner = strings.TrimLeft(owner, "@")
	return strings.ToLower(owner)
}

func (c ClawHubConfig) IsEnabled() bool {
	return c.Enabled
}

func (c ClawHubConfig) BuildRegistry() SkillRegistry {
	return NewClawHubRegistry(c)
}

// --- Search ---

type clawhubSearchResponse struct {
	Results []clawhubSearchResult `json:"results"`
}

type clawhubSearchResult struct {
	Score        float64 `json:"score"`
	Slug         *string `json:"slug"`
	DisplayName  *string `json:"displayName"`
	Summary      *string `json:"summary"`
	Version      *string `json:"version"`
	OwnerHandle  *string `json:"ownerHandle"`
	CanonicalURL *string `json:"canonicalUrl"`
	Downloads    *int64  `json:"downloads"`

	Install *struct {
		Kind      string `json:"kind"`
		Reference string `json:"reference"`
		SourceURL string `json:"sourceUrl"`
	} `json:"install"`

	Links *struct {
		Canonical string `json:"canonical"`
		Source    string `json:"source"`
	} `json:"links"`

	Trust *struct {
		Installability string `json:"installability"`
	} `json:"trust"`

	Metrics *struct {
		Rolling60DayInstalls int64 `json:"rolling60DayInstalls"`
	} `json:"metrics"`

	Native *struct {
		OwnerHandle string `json:"ownerHandle"`
	} `json:"native"`

	SourceIdentity *struct {
		Owner string `json:"owner"`
	} `json:"sourceIdentity"`
}

func (c *ClawHubRegistry) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	u, err := url.Parse(c.baseURL + c.searchPath)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	u.RawQuery = q.Encode()

	body, err := c.doGet(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var resp clawhubSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	results := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		slug := utils.DerefStr(r.Slug, "")
		if slug == "" {
			continue
		}

		summary := utils.DerefStr(r.Summary, "")
		if summary == "" {
			continue
		}

		displayName := utils.DerefStr(r.DisplayName, "")
		if displayName == "" {
			displayName = slug
		}

		ownerHandle := utils.DerefStr(r.OwnerHandle, "")
		if ownerHandle == "" && r.Native != nil {
			ownerHandle = r.Native.OwnerHandle
		}
		if ownerHandle == "" && r.SourceIdentity != nil {
			ownerHandle = r.SourceIdentity.Owner
		}

		installKind := ""
		installReference := ""
		sourceURL := ""
		if r.Install != nil {
			installKind = r.Install.Kind
			installReference = r.Install.Reference
			sourceURL = r.Install.SourceURL
		}

		canonicalURL := utils.DerefStr(r.CanonicalURL, "")
		if canonicalURL == "" && r.Links != nil {
			canonicalURL = r.Links.Canonical
		}
		canonicalURL = c.absoluteURL(canonicalURL)
		if canonicalURL == "" {
			canonicalURL = c.SkillURLForOwner(slug, ownerHandle, utils.DerefStr(r.Version, ""))
		}

		downloads := int64(0)
		if r.Downloads != nil {
			downloads = *r.Downloads
		} else if r.Metrics != nil {
			downloads = r.Metrics.Rolling60DayInstalls
		}

		installability := ""
		if r.Trust != nil {
			installability = r.Trust.Installability
		}

		results = append(results, SearchResult{
			Score:            r.Score,
			Slug:             slug,
			DisplayName:      displayName,
			Summary:          summary,
			Version:          utils.DerefStr(r.Version, ""),
			RegistryName:     c.Name(),
			OwnerHandle:      ownerHandle,
			InstallKind:      installKind,
			InstallReference: installReference,
			CanonicalURL:     canonicalURL,
			SourceURL:        sourceURL,
			Downloads:        downloads,
			Installability:   installability,
		})
	}

	return results, nil
}

func (c *ClawHubRegistry) absoluteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return raw
	}
	return strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(raw, "/")
}

// --- GetSkillMeta ---

type clawhubSkillResponse struct {
	Skill *struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"displayName"`
		Summary     string `json:"summary"`
	} `json:"skill"`
	Owner         *clawhubOwnerInfo      `json:"owner"`
	LatestVersion *clawhubVersionInfo    `json:"latestVersion"`
	Moderation    *clawhubModerationInfo `json:"moderation"`

	// Legacy flat shape (older ClawHub deployments).
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary"`
}

type clawhubOwnerInfo struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
}

type clawhubVersionInfo struct {
	Version string `json:"version"`
}

type clawhubModerationInfo struct {
	IsMalwareBlocked bool     `json:"isMalwareBlocked"`
	IsSuspicious     bool     `json:"isSuspicious"`
	Verdict          string   `json:"verdict"`
	ReasonCodes      []string `json:"reasonCodes"`
	Summary          string   `json:"summary"`
}

type clawhubAmbiguousResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Slug    string `json:"slug"`
	Matches []struct {
		OwnerHandle string `json:"ownerHandle"`
		Slug        string `json:"slug"`
		Ref         string `json:"ref"`
		URL         string `json:"url"`
	} `json:"matches"`
}

func (c *ClawHubRegistry) GetSkillMeta(ctx context.Context, slug string) (*SkillMeta, error) {
	return c.GetSkillMetaForOwner(ctx, slug, "")
}

// GetSkillMetaForOwner fetches metadata for one publisher's skill.
func (c *ClawHubRegistry) GetSkillMetaForOwner(ctx context.Context, slug, ownerHandle string) (*SkillMeta, error) {
	if err := utils.ValidateSkillIdentifier(slug); err != nil {
		return nil, fmt.Errorf("invalid slug %q: error: %s", slug, err.Error())
	}

	u, err := url.Parse(c.baseURL + c.skillsPath + "/" + url.PathEscape(slug))
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if owner := normalizeOwnerHandle(ownerHandle); owner != "" {
		q := u.Query()
		q.Set("ownerHandle", owner)
		u.RawQuery = q.Encode()
	}

	body, status, err := c.doGetWithStatus(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("skill metadata request failed: %w", err)
	}
	if status == http.StatusConflict {
		return nil, parseClawHubAmbiguousError(slug, body)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("skill metadata request failed: HTTP %d: %s", status, string(body))
	}

	var resp clawhubSkillResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse skill metadata: %w", err)
	}

	meta := &SkillMeta{RegistryName: c.Name()}
	if resp.Skill != nil {
		meta.Slug = resp.Skill.Slug
		meta.DisplayName = resp.Skill.DisplayName
		meta.Summary = resp.Skill.Summary
	}
	if meta.Slug == "" {
		meta.Slug = resp.Slug
	}
	if meta.DisplayName == "" {
		meta.DisplayName = resp.DisplayName
	}
	if meta.Summary == "" {
		meta.Summary = resp.Summary
	}
	if resp.Owner != nil {
		meta.OwnerHandle = normalizeOwnerHandle(resp.Owner.Handle)
	}
	if meta.OwnerHandle == "" {
		meta.OwnerHandle = normalizeOwnerHandle(ownerHandle)
	}
	if meta.OwnerHandle != "" {
		meta.CanonicalURL = c.SkillURLForOwner(slug, meta.OwnerHandle, "")
	}

	if resp.LatestVersion != nil {
		meta.LatestVersion = resp.LatestVersion.Version
	}
	if resp.Moderation != nil {
		meta.IsMalwareBlocked = resp.Moderation.IsMalwareBlocked
		meta.IsSuspicious = resp.Moderation.IsSuspicious ||
			strings.EqualFold(resp.Moderation.Verdict, "suspicious")
	}

	return meta, nil
}

func parseClawHubAmbiguousError(slug string, body []byte) error {
	var resp clawhubAmbiguousResponse
	_ = json.Unmarshal(body, &resp)
	err := &AmbiguousSkillSlugError{Slug: slug}
	for _, match := range resp.Matches {
		err.Matches = append(err.Matches, SkillOwnerMatch{
			OwnerHandle: normalizeOwnerHandle(match.OwnerHandle),
			Slug:        match.Slug,
			Ref:         match.Ref,
			URL:         match.URL,
		})
	}
	return err
}

// --- DownloadAndInstall ---

// DownloadAndInstall fetches metadata (with fallback), resolves version,
// downloads the skill artifact, and extracts it to targetDir.
func (c *ClawHubRegistry) DownloadAndInstall(
	ctx context.Context,
	slug, version, targetDir string,
) (*InstallResult, error) {
	return c.DownloadAndInstallForOwner(ctx, slug, version, "", targetDir)
}

// DownloadAndInstallForOwner is the publisher-scoped install path.
func (c *ClawHubRegistry) DownloadAndInstallForOwner(
	ctx context.Context,
	slug, version, ownerHandle, targetDir string,
) (*InstallResult, error) {
	if err := utils.ValidateSkillIdentifier(slug); err != nil {
		return nil, fmt.Errorf("invalid slug %q: error: %s", slug, err.Error())
	}

	owner := normalizeOwnerHandle(ownerHandle)

	// Step 1: Fetch metadata (with fallback), but keep ambiguity errors.
	result := &InstallResult{}
	meta, err := c.GetSkillMetaForOwner(ctx, slug, owner)
	if err != nil {
		var ambiguous *AmbiguousSkillSlugError
		if errors.As(err, &ambiguous) {
			return nil, ambiguous
		}
		meta = nil
	}

	if meta != nil {
		result.IsMalwareBlocked = meta.IsMalwareBlocked
		result.IsSuspicious = meta.IsSuspicious
		result.Summary = meta.Summary
		if owner == "" && meta.OwnerHandle != "" {
			owner = meta.OwnerHandle
		}
	}

	// Step 2: Resolve version.
	installVersion := version
	if installVersion == "" && meta != nil {
		installVersion = meta.LatestVersion
	}
	if installVersion == "" {
		installVersion = "latest"
	}
	result.Version = installVersion

	// Step 3: Download the artifact (ZIP or JSON GitHub handoff).
	u, err := url.Parse(c.baseURL + c.downloadPath)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("slug", slug)
	if owner != "" {
		q.Set("ownerHandle", owner)
	}
	if installVersion != "latest" {
		q.Set("version", installVersion)
	}
	u.RawQuery = q.Encode()

	tmpPath, contentType, err := c.downloadToTempFileWithRetry(ctx, u.String(), slug)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpPath)

	// Step 4: Install from ZIP or GitHub handoff descriptor.
	if isJSONArtifact(tmpPath, contentType) {
		if err := c.installFromHandoff(ctx, tmpPath, targetDir); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := utils.ExtractZipFile(tmpPath, targetDir); err != nil {
		return nil, err
	}

	return result, nil
}

// --- GitHub handoff ---

func isJSONArtifact(filePath, contentType string) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	trimmed := bytes.TrimSpace(head[:n])
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func (c *ClawHubRegistry) installFromHandoff(
	ctx context.Context, handoffPath, targetDir string,
) error {
	data, err := os.ReadFile(handoffPath)
	if err != nil {
		return fmt.Errorf("failed to read GitHub handoff: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to parse GitHub handoff: %w", err)
	}
	if nested, ok := raw["handoff"].(map[string]any); ok {
		raw = nested
	}
	if nested, ok := raw["source"].(map[string]any); ok {
		raw = nested
	}

	archiveURL := handoffString(raw, "archiveUrl", "archive_url")
	if archiveURL != "" {
		parsed, err := url.Parse(archiveURL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return fmt.Errorf("GitHub handoff has an invalid archive URL")
		}
		tmpPath, contentType, err := c.downloadToTempFileWithRetry(ctx, archiveURL, "")
		if err != nil {
			return fmt.Errorf("failed to download GitHub archive: %w", err)
		}
		defer os.Remove(tmpPath)
		if isJSONArtifact(tmpPath, contentType) {
			return fmt.Errorf("GitHub handoff archive did not return a ZIP file")
		}
		if err := utils.ExtractZipFile(tmpPath, targetDir); err != nil {
			return err
		}
		return nil
	}

	repo := handoffString(raw, "repo", "repository", "repoUrl", "repo_url")
	if repo == "" {
		return fmt.Errorf("GitHub handoff does not include an archive URL or repository")
	}
	commit := handoffString(raw, "commit", "ref", "sha")
	subPath := handoffString(raw, "path", "subPath", "sub_path")

	installTarget := repo
	if subPath != "" {
		installTarget = strings.TrimRight(repo, "/") + "/" + strings.Trim(subPath, "/")
	}

	installer, err := newClawHubHandoffInstaller()
	if err != nil {
		return fmt.Errorf("failed to create GitHub installer: %w", err)
	}
	if _, err := installer.InstallFromGitHubToDir(ctx, installTarget, commit, targetDir); err != nil {
		return fmt.Errorf("failed to install GitHub handoff: %w", err)
	}
	return nil
}

// newClawHubHandoffInstaller is a test seam for GitHub-backed handoff installs.
var newClawHubHandoffInstaller = func() (*SkillInstaller, error) {
	return NewSkillInstallerWithBaseURL("", "", "", "")
}

func handoffString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

type clawhubCatalogResponse struct {
	Items []struct {
		Slug          string              `json:"slug"`
		DisplayName   string              `json:"displayName"`
		Summary       string              `json:"summary"`
		OwnerHandle   string              `json:"ownerHandle"`
		LatestVersion *clawhubVersionInfo `json:"latestVersion"`
		Stats         *struct {
			Downloads int64 `json:"downloads"`
		} `json:"stats"`
	} `json:"items"`
	NextCursor string `json:"nextCursor"`
}

// PopularSkills browses the ClawHub catalog sorted by lifetime downloads.
// Cursor pagination follows the registry's nextCursor value.
func (c *ClawHubRegistry) PopularSkills(
	ctx context.Context, limit int, cursor string,
) ([]SearchResult, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	u, err := url.Parse(c.baseURL + c.skillsPath)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base URL: %w", err)
	}
	q := u.Query()
	q.Set("sort", "downloads")
	q.Set("limit", strconv.Itoa(limit))
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()

	body, err := c.doGet(ctx, u.String())
	if err != nil {
		return nil, "", fmt.Errorf("catalog request failed: %w", err)
	}

	var resp clawhubCatalogResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("failed to parse catalog response: %w", err)
	}

	results := make([]SearchResult, 0, len(resp.Items))
	for _, item := range resp.Items {
		if strings.TrimSpace(item.Slug) == "" {
			continue
		}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = item.Slug
		}
		owner := normalizeOwnerHandle(item.OwnerHandle)
		downloads := int64(0)
		if item.Stats != nil {
			downloads = item.Stats.Downloads
		}
		version := ""
		if item.LatestVersion != nil {
			version = item.LatestVersion.Version
		}
		results = append(results, SearchResult{
			Slug:             item.Slug,
			DisplayName:      displayName,
			Summary:          strings.TrimSpace(item.Summary),
			Version:          version,
			RegistryName:     c.Name(),
			OwnerHandle:      owner,
			InstallKind:      "clawhub",
			InstallReference: strings.TrimPrefix(owner+"/"+item.Slug, "/"),
			CanonicalURL:     c.SkillURLForOwner(item.Slug, owner, version),
			Downloads:        downloads,
			Installability:   "installable",
		})
	}
	return results, resp.NextCursor, nil
}

// --- Security summary ---

const clawhubSecurityCacheTTL = 5 * time.Minute

type clawhubSecurityCacheEntry struct {
	security *SkillSecurity
	expires  time.Time
}

var clawhubSecurityCache sync.Map // key: owner/slug

func resetClawHubSecurityCache() {
	clawhubSecurityCache.Range(func(key, _ any) bool {
		clawhubSecurityCache.Delete(key)
		return true
	})
}

type clawhubScanResponse struct {
	Skill *struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"displayName"`
	} `json:"skill"`
	Version *struct {
		Version string `json:"version"`
	} `json:"version"`
	Moderation *clawhubModerationInfo `json:"moderation"`
	Security   *struct {
		Status        string `json:"status"`
		HasScanResult bool   `json:"hasScanResult"`
		Scanners      *struct {
			SkillSpector *struct {
				Status     string  `json:"status"`
				Severity   string  `json:"severity"`
				Score      float64 `json:"score"`
				IssueCount int     `json:"issueCount"`
			} `json:"skillspector"`
			LLM *struct {
				Status   string `json:"status"`
				Verdict  string `json:"verdict"`
				Summary  string `json:"summary"`
				Guidance string `json:"guidance"`
			} `json:"llm"`
		} `json:"scanners"`
	} `json:"security"`
}

// GetSecuritySummary returns the normalized ClawHub audit verdict for a skill.
// Results are cached briefly to keep search responses fast.
func (c *ClawHubRegistry) GetSecuritySummary(
	ctx context.Context, slug, ownerHandle string,
) (*SkillSecurity, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, nil
	}
	owner := normalizeOwnerHandle(ownerHandle)
	cacheKey := owner + "/" + slug
	if cached, ok := clawhubSecurityCache.Load(cacheKey); ok {
		if entry, ok := cached.(clawhubSecurityCacheEntry); ok && time.Now().Before(entry.expires) {
			return entry.security, nil
		}
	}

	u, err := url.Parse(c.baseURL + c.skillsPath + "/" + url.PathEscape(slug) + "/scan")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	q := u.Query()
	q.Set("tag", "latest")
	if owner != "" {
		q.Set("ownerHandle", owner)
	}
	u.RawQuery = q.Encode()

	body, status, err := c.doGetWithStatus(ctx, u.String())
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("security scan request failed: HTTP %d: %s", status, string(body))
	}

	var resp clawhubScanResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse security scan: %w", err)
	}

	security := &SkillSecurity{
		Decision: "unknown",
		Status:   "unknown",
	}
	if resp.Security != nil {
		security.Status = strings.ToLower(strings.TrimSpace(resp.Security.Status))
		switch security.Status {
		case "clean", "benign":
			security.Decision = "pass"
			security.Status = "clean"
		case "suspicious", "warn", "warning":
			security.Decision = "warn"
			security.Status = "suspicious"
		case "malicious", "fail", "blocked":
			security.Decision = "fail"
			security.Status = "malicious"
		}
		if resp.Security.Scanners != nil {
			if resp.Security.Scanners.SkillSpector != nil {
				security.RiskLevel = strings.ToUpper(strings.TrimSpace(
					resp.Security.Scanners.SkillSpector.Severity,
				))
			}
			if resp.Security.Scanners.LLM != nil {
				security.Summary = strings.TrimSpace(resp.Security.Scanners.LLM.Summary)
				if security.Summary == "" {
					security.Summary = strings.TrimSpace(resp.Security.Scanners.LLM.Guidance)
				}
			}
		}
	}
	if resp.Moderation != nil {
		if resp.Moderation.IsMalwareBlocked {
			security.Decision = "fail"
			security.Status = "malicious"
		} else if resp.Moderation.IsSuspicious && security.Decision == "pass" {
			security.Decision = "warn"
			security.Status = "suspicious"
		}
		if security.Summary == "" {
			security.Summary = strings.TrimSpace(resp.Moderation.Summary)
		}
	}
	if security.RiskLevel == "" {
		switch security.Decision {
		case "pass":
			security.RiskLevel = "LOW"
		case "warn":
			security.RiskLevel = "MEDIUM"
		case "fail":
			security.RiskLevel = "CRITICAL"
		}
	}

	version := ""
	if resp.Version != nil {
		version = strings.TrimSpace(resp.Version.Version)
	}
	auditURL := c.SkillURLForOwner(slug, owner, version)
	if auditURL != "" {
		auditURL += "/security-audit"
		if version != "" {
			auditURL += "?version=" + url.QueryEscape(version)
		}
	}
	security.AuditURL = auditURL

	clawhubSecurityCache.Store(cacheKey, clawhubSecurityCacheEntry{
		security: security,
		expires:  time.Now().Add(clawhubSecurityCacheTTL),
	})
	return security, nil
}

// --- HTTP helper ---

func (c *ClawHubRegistry) doGet(ctx context.Context, urlStr string) ([]byte, error) {
	body, status, err := c.doGetWithStatus(ctx, urlStr)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", status, string(body))
	}
	return body, nil
}

// doGetWithStatus returns the body even for non-2xx responses so callers can
// parse structured errors (for example ambiguous slug responses).
func (c *ClawHubRegistry) doGetWithStatus(ctx context.Context, urlStr string) ([]byte, int, error) {
	req, err := c.newGetRequest(ctx, urlStr, "application/json")
	if err != nil {
		return nil, 0, err
	}

	resp, err := utils.DoRequestWithRetry(c.client, req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Limit response body read to prevent memory issues.
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxResponseSize)))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

func (c *ClawHubRegistry) newGetRequest(ctx context.Context, urlStr, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return req, nil
}

func (c *ClawHubRegistry) downloadToTempFileWithRetry(
	ctx context.Context, urlStr, slug string,
) (string, string, error) {
	req, err := c.newGetRequest(ctx, urlStr, "application/zip, application/json;q=0.9, */*;q=0.8")
	if err != nil {
		return "", "", err
	}

	resp, err := utils.DoRequestWithRetry(c.client, req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody := make([]byte, 1024)
		n, _ := io.ReadFull(resp.Body, errBody)
		if resp.StatusCode == http.StatusConflict {
			return "", "", &AmbiguousSkillSlugError{Slug: slug}
		}
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody[:n]))
	}

	tmpFile, err := os.CreateTemp("", "picoclaw-dl-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	src := io.LimitReader(resp.Body, int64(c.maxZipSize)+1)
	written, err := io.Copy(tmpFile, src)
	if err != nil {
		cleanup()
		return "", "", fmt.Errorf("download write failed: %w", err)
	}

	if written > int64(c.maxZipSize) {
		cleanup()
		return "", "", fmt.Errorf("download too large: %d bytes (max %d)", written, c.maxZipSize)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tmpPath, resp.Header.Get("Content-Type"), nil
}
