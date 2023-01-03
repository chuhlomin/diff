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
)

type server struct {
	router   chi.Router
	tags     []tag
	contents map[string]map[string]string // tag -> file -> content
	patches  []patch
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

	// serve static directories
	s.router.Get("/vs/*", http.StripPrefix("/vs", http.FileServer(http.Dir("static/vs"))).ServeHTTP)
	s.router.Get("/min-maps/*", http.StripPrefix("/min-maps", http.FileServer(http.Dir("static/min-maps"))).ServeHTTP)

	s.router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		// get tags and sort them descending by value
		tags := make([]string, 0, len(s.tags))
		for _, t := range s.tags {
			tags = append(tags, t.Name)
		}

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
	Name       string
	ChangeType string // A, D, M, R
	OldName    string
}

func (f file) Less(other file) bool {
	return f.Name < other.Name
}

func (s *server) handlerFiles(w http.ResponseWriter, r *http.Request) {
	tag1 := chi.URLParam(r, "tag1")
	tag2 := chi.URLParam(r, "tag2")

	if tag1 == tag2 {
		http.Error(w, "tags must be different", http.StatusBadRequest)
		return
	}

	// log.Printf("Collecting changes between %s and %s", tag1, tag2)

	// collect all changes between the two tags
	changes := map[string]file{}

	collecting := false
	for _, patch := range s.patches {
		if collecting {
			// log.Printf("%s -> %s (%d)", patch.from, patch.to, len(patch.changes))

			for _, fileChange := range patch.changes {
				if fileChange.IsBinary() {
					continue
				}

				from, to := fileChange.Files()

				if from == nil {
					changes[to.Path()] = file{
						Name:       to.Path(),
						ChangeType: "A",
					}
					continue
				}

				if to == nil {
					changes[from.Path()] = file{
						Name:       from.Path(),
						ChangeType: "D",
					}
					continue
				}

				if from.Path() != to.Path() {
					changes[to.Path()] = file{
						Name:       to.Path(),
						ChangeType: "R",
						OldName:    from.Path(),
					}
					continue
				}

				if from.Path() == to.Path() {
					changes[to.Path()] = file{
						Name:       to.Path(),
						ChangeType: "M",
					}
					continue
				}
			}
		}

		if patch.to == tag1 || patch.to == tag2 {
			// log.Printf("Collecting toggle by tag %s", patch.to)
			collecting = !collecting
			if !collecting {
				break
			}
		}
	}

	if collecting {
		// did not find the second tag, return error
		http.Error(w, fmt.Sprintf("tag %s or %s not found", tag2, tag1), http.StatusNotFound)
		return
	}

	changesList := make([]file, 0, len(changes))
	// log.Printf("Changes: %d", len(changes))
	for _, change := range changes {
		// log.Printf("  %s %s", change.ChangeType, change.Name)
		changesList = append(changesList, change)
	}

	sort.Slice(changesList, func(i, j int) bool {
		return changesList[i].Name < changesList[j].Name
	})

	// render diff template
	if err := s.tmpl.ExecuteTemplate(w, "files.gohtml", struct {
		Tag1    string
		Tag2    string
		Changes []file
	}{
		Tag1:    tag1,
		Tag2:    tag2,
		Changes: changesList,
	}); err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handlerDiff(w http.ResponseWriter, r *http.Request) {
	tag1 := r.URL.Query().Get("tag1")
	tag2 := r.URL.Query().Get("tag2")
	file := r.URL.Query().Get("file")
	oldFile := r.URL.Query().Get("oldFile")

	// render diff template
	if err := s.tmpl.ExecuteTemplate(w, "diff.gohtml", struct {
		Tag1    string
		Tag2    string
		File    string
		OldFile string
	}{
		Tag1:    tag1,
		Tag2:    tag2,
		File:    file,
		OldFile: oldFile,
	}); err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handlerVersions(w http.ResponseWriter, r *http.Request) {
	tag1 := strings.Trim(r.URL.Query().Get("tag1"), "\"")
	tag2 := strings.Trim(r.URL.Query().Get("tag2"), "\"")
	file := strings.Trim(r.URL.Query().Get("file"), "\"")
	oldFile := strings.Trim(r.URL.Query().Get("oldfile"), "\"")
	if oldFile == "" {
		oldFile = file
	}

	// log.Printf("Getting content for %s (%s) -> %s (%s)", tag1, oldFile, tag2, file)

	var (
		content1, content2 string
		err                error
	)

	if _, ok := s.contents[tag1][oldFile]; ok {
		content1, err = s.getFileContent(tag1, oldFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not get file content for tag %s: %v", tag1, err), http.StatusInternalServerError)
			return
		}
	}

	if _, ok := s.contents[tag2][file]; ok {
		content2, err = s.getFileContent(tag2, file)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not get file content for tag %s: %v", tag2, err), http.StatusInternalServerError)
			return
		}
	}

	if content1 == "" && content2 == "" {
		log.Printf("File content not found in tags %s (%s) and %s (%s)", tag2, file, tag1, oldFile)
		http.Error(w, fmt.Sprintf("file content not found in tags %s (%s) and %s (%s)", tag2, file, tag1, oldFile), http.StatusNotFound)
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
