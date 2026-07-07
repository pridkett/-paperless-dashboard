// Package paperless is a minimal client for the Paperless-NGX REST API,
// covering just what the dashboard needs: resolving correspondent, document
// type, tag, and custom field names to IDs, and listing documents that match
// a filter within a date range.
package paperless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client talks to one Paperless-NGX instance.
type Client struct {
	baseURL string
	token   string
	http    *http.Client

	mu             sync.Mutex
	correspondents map[string]int // lowercase name -> id
	documentTypes  map[string]int
	tags           map[string]int
	customFields   map[string]int
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Document is the subset of document fields the dashboard uses.
type Document struct {
	ID           int               `json:"id"`
	Title        string            `json:"title"`
	Created      string            `json:"created"` // RFC 3339 or YYYY-MM-DD depending on version
	Tags         []int             `json:"tags"`
	CustomFields []CustomFieldItem `json:"custom_fields"`
}

// CustomFieldItem is a custom field value attached to a document.
type CustomFieldItem struct {
	Field int             `json:"field"`
	Value json.RawMessage `json:"value"`
}

// CreatedTime parses the document's created timestamp.
func (d Document) CreatedTime() (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, d.Created); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", d.Created)
}

type listPage[T any] struct {
	Count   int    `json:"count"`
	Next    string `json:"next"`
	Results []T    `json:"results"`
}

type namedObject struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Ping verifies connectivity and credentials.
func (c *Client) Ping(ctx context.Context) error {
	var page listPage[json.RawMessage]
	return c.getJSON(ctx, "/api/documents/?page_size=1", &page)
}

// Filter selects documents for one check. Zero/nil fields are omitted.
type Filter struct {
	CorrespondentID int
	DocumentTypeID  int
	TagIDs          []int
	CreatedFrom     time.Time
	CreatedTo       time.Time
}

// Documents returns all documents matching the filter, following pagination.
func (c *Client) Documents(ctx context.Context, f Filter) ([]Document, error) {
	q := url.Values{}
	q.Set("page_size", "250")
	q.Set("ordering", "-created")
	if f.CorrespondentID != 0 {
		q.Set("correspondent__id", strconv.Itoa(f.CorrespondentID))
	}
	if f.DocumentTypeID != 0 {
		q.Set("document_type__id", strconv.Itoa(f.DocumentTypeID))
	}
	if len(f.TagIDs) > 0 {
		ids := make([]string, len(f.TagIDs))
		for i, id := range f.TagIDs {
			ids[i] = strconv.Itoa(id)
		}
		q.Set("tags__id__all", strings.Join(ids, ","))
	}
	if !f.CreatedFrom.IsZero() {
		q.Set("created__date__gte", f.CreatedFrom.Format("2006-01-02"))
	}
	if !f.CreatedTo.IsZero() {
		q.Set("created__date__lte", f.CreatedTo.Format("2006-01-02"))
	}

	path := "/api/documents/?" + q.Encode()
	var docs []Document
	for path != "" {
		var page listPage[Document]
		if err := c.getJSON(ctx, path, &page); err != nil {
			return nil, err
		}
		docs = append(docs, page.Results...)
		path = c.relativize(page.Next)
	}
	return docs, nil
}

// CorrespondentID resolves a correspondent name (case-insensitive) to its ID.
func (c *Client) CorrespondentID(ctx context.Context, name string) (int, error) {
	return c.lookup(ctx, "/api/correspondents/", "correspondent", &c.correspondents, name)
}

// DocumentTypeID resolves a document type name to its ID.
func (c *Client) DocumentTypeID(ctx context.Context, name string) (int, error) {
	return c.lookup(ctx, "/api/document_types/", "document type", &c.documentTypes, name)
}

// TagID resolves a tag name to its ID.
func (c *Client) TagID(ctx context.Context, name string) (int, error) {
	return c.lookup(ctx, "/api/tags/", "tag", &c.tags, name)
}

// CustomFieldID resolves a custom field name to its ID.
func (c *Client) CustomFieldID(ctx context.Context, name string) (int, error) {
	return c.lookup(ctx, "/api/custom_fields/", "custom field", &c.customFields, name)
}

// lookup fetches and caches the full name->ID table for one object kind,
// then answers from the cache. Paperless instances rarely have more than a
// few hundred of any of these, so one paginated fetch is cheaper and simpler
// than per-name filtered queries.
func (c *Client) lookup(ctx context.Context, path, kind string, cache *map[string]int, name string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if *cache == nil {
		table := make(map[string]int)
		next := path + "?page_size=250"
		for next != "" {
			var page listPage[namedObject]
			if err := c.getJSON(ctx, next, &page); err != nil {
				return 0, err
			}
			for _, obj := range page.Results {
				table[strings.ToLower(obj.Name)] = obj.ID
			}
			next = c.relativize(page.Next)
		}
		*cache = table
	}

	id, ok := (*cache)[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("no %s named %q in Paperless", kind, name)
	}
	return id, nil
}

// InvalidateCaches drops the name->ID caches so renames in Paperless are
// picked up on the next refresh.
func (c *Client) InvalidateCaches() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.correspondents, c.documentTypes, c.tags, c.customFields = nil, nil, nil, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("paperless returned %s for %s: %s", resp.Status, path, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// relativize converts Paperless's absolute "next page" URL into a path
// relative to the configured base URL, so requests keep working when the
// server is behind a proxy that rewrites hosts.
func (c *Client) relativize(next string) string {
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return ""
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}
