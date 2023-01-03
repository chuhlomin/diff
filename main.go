package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
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

	log.Printf("Getting diffs")
	patches, err := getDiff(repo, tags)
	if err != nil {
		return fmt.Errorf("get renames: %w", err)
	}

	log.Printf("Getting files per tag")
	contents, err := getContentsPerTag(repo, tags)
	if err != nil {
		return fmt.Errorf("get files per tag: %w", err)
	}

	log.Printf("Processing templates")
	tmpl, err := template.New("").ParseGlob("templates/*.gohtml")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	sort.Slice(tags, func(i, j int) bool {
		// tag has a format "2.10-v3877"
		// we want to sort by "v3877" part descending
		return strings.Split(tags[i].Name, "-")[1] > strings.Split(tags[j].Name, "-")[1]
	})

	server := server{
		router:   chi.NewRouter(),
		tags:     tags,
		contents: contents,
		patches:  patches,
		tmpl:     tmpl,
	}
	server.routes()

	log.Printf("Listening on :8080")
	return http.ListenAndServe(":8080", &server)
}

type tag struct {
	Name string
	Hash plumbing.Hash
}

func getTags(r *git.Repository) ([]tag, error) {
	var tags []tag

	refs, err := r.Tags()
	if err != nil {
		return nil, fmt.Errorf("get tags: %w", err)
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		t := strings.TrimLeft(ref.Name().String(), "refs/tags/")
		tags = append(tags, tag{
			Name: t,
			Hash: ref.Hash(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	sort.Slice(tags, func(i, j int) bool {
		// tag has a format "2.10-v3877"
		// we want to sort by "v3877" part descending
		return strings.Split(tags[i].Name, "-")[1] > strings.Split(tags[j].Name, "-")[1]
	})

	return tags, nil
}

func getContentsPerTag(r *git.Repository, tags []tag) (map[string]map[string]string, error) {
	contents := map[string]map[string]string{} // tag -> file -> content

	// get all files in the tag
	for _, tag := range tags {
		contents[tag.Name] = map[string]string{}

		commit, err := r.CommitObject(tag.Hash)
		if err != nil {
			return nil, fmt.Errorf("get commit for tag %q: %w", tag, err)
		}

		tree, err := commit.Tree()
		if err != nil {
			return nil, fmt.Errorf("get tree: %w", err)
		}

		err = tree.Files().ForEach(func(file *object.File) error {
			content, err := file.Contents()
			if err != nil {
				return fmt.Errorf("get file content: %w", err)
			}
			contents[tag.Name][file.Name] = content

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("iterate files: %w", err)
		}
	}

	return contents, nil
}

type patch struct {
	from     string
	to       string
	fromHash string
	toHash   string
	changes  []diff.FilePatch
}

func getDiff(r *git.Repository, tags []tag) ([]patch, error) {
	var result []patch

	sort.Slice(tags, func(i, j int) bool {
		// tag has a format "2.10-v3877"
		// we want to sort by "v3877" part acsending
		return strings.Split(tags[i].Name, "-")[1] < strings.Split(tags[j].Name, "-")[1]
	})

	var commitPrev *object.Commit
	var tagPrev string
	for _, tag := range tags {
		commit, err := r.CommitObject(tag.Hash)
		if err != nil {
			return nil, fmt.Errorf("get commit for tag %q: %w", tag.Name, err)
		}

		if commitPrev == nil {
			result = append(result, patch{
				from:   tagPrev,
				to:     tag.Name,
				toHash: commit.Hash.String(),
			})
			commitPrev = commit
			tagPrev = tag.Name
			continue
		}

		p, err := commitPrev.Patch(commit)
		if err != nil {
			return nil, fmt.Errorf("get patch: %w", err)
		}

		result = append(result, patch{
			from:     tagPrev,
			to:       tag.Name,
			fromHash: commitPrev.Hash.String(),
			toHash:   commit.Hash.String(),
			changes:  p.FilePatches(),
		})
		tagPrev = tag.Name
		commitPrev = commit
	}

	return result, nil
}
