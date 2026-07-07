// Package web serves the dashboard: a single server-rendered page backed by
// a periodically refreshed snapshot of check results.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pridkett/paperless-dashboard/internal/checks"
	"github.com/pridkett/paperless-dashboard/internal/config"
)

//go:embed templates/*.html static/*
var assets embed.FS

// Server renders the dashboard and keeps its snapshot fresh.
type Server struct {
	engine  *checks.Engine
	cfg     *config.Config
	tmpl    *template.Template
	refresh time.Duration

	mu      sync.RWMutex
	results []checks.Result
	updated time.Time
}

func New(engine *checks.Engine, cfg *config.Config) *Server {
	base := strings.TrimRight(cfg.URL.Value, "/")
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("Jan 2, 2006")
		},
		// docURL links to a document's detail page in the Paperless-NGX web UI.
		"docURL": func(id int) string {
			return fmt.Sprintf("%s/documents/%d/details", base, id)
		},
	}).ParseFS(assets, "templates/*.html"))

	return &Server{
		engine:  engine,
		cfg:     cfg,
		tmpl:    tmpl,
		refresh: time.Duration(cfg.RefreshMinutes) * time.Minute,
	}
}

// Run performs the initial evaluation, starts the background refresher, and
// serves HTTP until ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	s.update(ctx)
	go s.refreshLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("POST /refresh", s.handleRefresh)
	mux.Handle("GET /static/", http.FileServerFS(assets))

	srv := &http.Server{Addr: s.cfg.Listen.Value, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(s.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.engine.Client.InvalidateCaches()
			s.update(ctx)
		}
	}
}

func (s *Server) update(ctx context.Context) {
	results := s.engine.Evaluate(ctx, s.cfg.Checks, time.Now())
	s.mu.Lock()
	s.results, s.updated = results, time.Now()
	s.mu.Unlock()
}

type pageData struct {
	Results      []checks.Result
	Updated      time.Time
	PaperlessURL string
	Healthy      int
	Attention    int
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	data := pageData{
		Results:      s.results,
		Updated:      s.updated,
		PaperlessURL: s.cfg.URL.Value,
	}
	s.mu.RUnlock()

	for _, res := range data.Results {
		if res.Healthy() {
			data.Healthy++
		} else {
			data.Attention++
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("render: %v", err)
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	s.engine.Client.InvalidateCaches()
	s.update(r.Context())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
