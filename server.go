package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-git/go-git/v5/plumbing"
)

type server struct {
	router   chi.Router
	tags     map[string]plumbing.Hash            // tag -> hash
	files    map[string]map[string]plumbing.Hash // tag -> file -> hash
	contents map[string]map[string]string        // tag -> file -> content
	tmpl     *template.Template
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *server) routes() {
	s.router.Use(middleware.StripSlashes)

	files, err := os.ReadDir("static")
	if err != nil {
		log.Fatalf("could not read static files: %v", err)
	}
	for _, file := range files {
		s.router.Get("/"+file.Name(), handlerStatic("static", file.Name()))
	}

	// serve static/vs directory
	s.router.Get("/vs/*", http.StripPrefix("/vs", http.FileServer(http.Dir("static/vs"))).ServeHTTP)
	s.router.Get("/min-maps/*", http.StripPrefix("/min-maps", http.FileServer(http.Dir("static/min-maps"))).ServeHTTP)

	s.router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		// get tags and sort them descending by value
		tags := make([]string, 0, len(s.tags))
		for tag := range s.tags {
			tags = append(tags, tag)
		}

		sort.Slice(tags, func(i, j int) bool {
			// tag has a format "2.10-v3877"
			// we want to sort by "v3877" part descending
			return strings.Split(tags[i], "-")[1] > strings.Split(tags[j], "-")[1]
		})

		// render index template
		if err := s.tmpl.ExecuteTemplate(w, "index.gohtml", struct {
			Tags []string
		}{
			Tags: tags,
		}); err != nil {
			log.Printf("Error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	s.router.Get("/files/{tag1}/{tag2}", s.handlerFiles)
	s.router.Get("/diff", s.handlerDiff)
	s.router.Get("/versions", s.handlerVersions)
}

type file struct {
	Name    string
	OldHash string
	NewHash string
}

func (f file) Less(other file) bool {
	return f.Name < other.Name
}

func (s *server) handlerFiles(w http.ResponseWriter, r *http.Request) {
	tag1 := chi.URLParam(r, "tag1")
	tag2 := chi.URLParam(r, "tag2")

	files1, ok := s.files[tag1]
	if !ok {
		http.Error(w, fmt.Sprintf("tag %s not found", tag1), http.StatusNotFound)
		return
	}

	files2, ok := s.files[tag2]
	if !ok {
		http.Error(w, fmt.Sprintf("tag %s not found", tag2), http.StatusNotFound)
		return
	}

	var files []file

	for f := range files1 {
		if _, ok := files2[f]; !ok {
			files = append(files, file{
				Name:    f,
				OldHash: files1[f].String(),
			})
			continue
		}

		if files1[f] != files2[f] {
			files = append(files, file{
				Name:    f,
				OldHash: files1[f].String(),
				NewHash: files2[f].String(),
			})
		}
	}

	for f := range files2 {
		if _, ok := files1[f]; !ok {
			files = append(files, file{
				Name:    f,
				NewHash: files2[f].String(),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Less(files[j])
	})

	// render diff template
	if err := s.tmpl.ExecuteTemplate(w, "files.gohtml", struct {
		Tag1  string
		Tag2  string
		Files []file
	}{
		Tag1:  tag1,
		Tag2:  tag2,
		Files: files,
	}); err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handlerDiff(w http.ResponseWriter, r *http.Request) {
	tag1 := r.URL.Query().Get("tag1")
	tag2 := r.URL.Query().Get("tag2")
	file := r.URL.Query().Get("file")

	// render diff template
	if err := s.tmpl.ExecuteTemplate(w, "diff.gohtml", struct {
		Tag1 string
		Tag2 string
		File string
	}{
		Tag1: tag1,
		Tag2: tag2,
		File: file,
	}); err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handlerVersions(w http.ResponseWriter, r *http.Request) {
	tag1 := strings.Trim(r.URL.Query().Get("tag1"), "\"")
	tag2 := strings.Trim(r.URL.Query().Get("tag2"), "\"")
	file := strings.Trim(r.URL.Query().Get("file"), "\"")

	var (
		content1, content2 string
		err                error
	)

	if _, ok := s.files[tag1][file]; ok {
		content1, err = s.getFileContent(tag1, file)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not get file content for tag %s: %v", tag1, err), http.StatusInternalServerError)
			return
		}
	}

	if _, ok := s.files[tag2][file]; ok {
		content2, err = s.getFileContent(tag2, file)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not get file content for tag %s: %v", tag2, err), http.StatusInternalServerError)
			return
		}
	}

	if content1 == "" && content2 == "" {
		keys := make([]string, 0, len(s.files))
		for k := range s.files {
			log.Printf("Comparing %s and %s", k, tag1)
			if k == tag1 {
				log.Printf("found tag %s", tag1)
			}
			keys = append(keys, k)
		}
		_, ok := s.files[tag1]
		log.Printf("ok: %v, keys: %v", ok, keys)
		http.Error(w, fmt.Sprintf("file %s not found in tags %s and %s", file, tag1, tag2), http.StatusNotFound)
		return
	}

	// return json with file content
	if err := json.NewEncoder(w).Encode(struct {
		Content1 string `json:"content1"`
		Content2 string `json:"content2"`
	}{
		Content1: content1,
		Content2: content2,
	}); err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) getFileContent(tag, file string) (string, error) {
	// get file content from s.contents
	// s.contents is a map: tag -> file -> content
	if _, ok := s.contents[tag]; !ok {
		return "", fmt.Errorf("tag %s not found", tag)
	}

	content, ok := s.contents[tag][file]
	if !ok {
		return "", fmt.Errorf("file %s not found", file)
	}

	return content, nil
}

func handlerStatic(dir, file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, dir+"/"+file)
	}
}
