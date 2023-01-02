package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

const repoURL = "https://github.com/ilyabirman/Aegea-Comparisons"

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	log.Printf("Cloning %s", repoURL)
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: repoURL,
	})
	if err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	log.Printf("Getting tags")
	tags, err := getTags(repo)
	if err != nil {
		return fmt.Errorf("get tags: %w", err)
	}

	log.Printf("Getting files per tag")
	files, contents, err := getFilesPerTag(repo, tags)
	if err != nil {
		return fmt.Errorf("get files per tag: %w", err)
	}

	log.Printf("Processing templates")
	tmpl, err := template.New("").ParseGlob("templates/*.gohtml")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	server := server{
		router:   chi.NewRouter(),
		tags:     tags,
		files:    files,
		contents: contents,
		tmpl:     tmpl,
	}
	server.routes()

	log.Printf("Listening on :8080")
	return http.ListenAndServe(":8080", &server)
}

func getTags(r *git.Repository) (map[string]plumbing.Hash, error) {
	tags := map[string]plumbing.Hash{}

	refs, err := r.Tags()
	if err != nil {
		return nil, fmt.Errorf("get tags: %w", err)
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		tag := strings.TrimLeft(ref.Name().String(), "refs/tags/")
		tags[tag] = ref.Hash()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	return tags, nil
}

func getFilesPerTag(r *git.Repository, tags map[string]plumbing.Hash) (
	map[string]map[string]plumbing.Hash,
	map[string]map[string]string,
	error,
) {
	files := map[string]map[string]plumbing.Hash{} // tag -> file -> hash
	contents := map[string]map[string]string{}     // tag -> file -> content

	// get all files in the tag
	for tag, hash := range tags {
		files[tag] = map[string]plumbing.Hash{}
		contents[tag] = map[string]string{}

		commit, err := r.CommitObject(hash)
		if err != nil {
			return nil, nil, fmt.Errorf("get commit for tag %q: %w", tag, err)
		}

		tree, err := commit.Tree()
		if err != nil {
			return nil, nil, fmt.Errorf("get tree: %w", err)
		}

		err = tree.Files().ForEach(func(file *object.File) error {
			files[tag][file.Name] = file.Hash

			content, err := file.Contents()
			if err != nil {
				return fmt.Errorf("get file content: %w", err)
			}
			contents[tag][file.Name] = content

			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("iterate files: %w", err)
		}
	}

	return files, contents, nil
}
