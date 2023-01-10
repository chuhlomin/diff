package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type generator struct {
	repo *git.Repository
	tmpl *template.Template

	contents map[string]map[string]string // tag -> file -> content
}

func (g *generator) Run() error {
	log.Printf("Getting tags")
	tags, err := getTags(g.repo)
	if err != nil {
		return fmt.Errorf("get tags: %w", err)
	}

	// create output directory
	if err := os.MkdirAll("output", 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := g.renderIndex(tags); err != nil {
		return fmt.Errorf("render index: %w", err)
	}

	log.Printf("Getting diffs")
	forward, backward, err := getDiff(g.repo, tags)
	if err != nil {
		return fmt.Errorf("get renames: %w", err)
	}

	log.Printf("Rendering files changes")
	if err := g.renderFilesChanges(tags, forward, backward); err != nil {
		return fmt.Errorf("render files: %w", err)
	}

	log.Printf("Copying static files")
	if err := g.copyStaticFiles(); err != nil {
		return fmt.Errorf("copy static files: %w", err)
	}
	return nil
}

func (g *generator) renderIndex(tags []tag) error {
	// render index template into `output/index.html`
	f, err := os.Create("output/index.html")
	if err != nil {
		return fmt.Errorf("create index.html: %w", err)
	}
	defer f.Close()

	if err := g.tmpl.ExecuteTemplate(f, "index.gohtml", struct {
		Tags []tag
	}{
		Tags: tags,
	}); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

type file struct {
	Name       string
	ChangeType string // A, D, M, R
	OldName    string
}

func (f file) Less(other file) bool {
	return f.Name < other.Name
}

func (g *generator) renderFilesChanges(tags []tag, forward, backward []patch) error {
	// get all combinations of tags
	for _, tag1 := range tags {
		for _, tag2 := range tags {
			patches := forward
			if strings.Split(tag1.Name, "-")[0] < strings.Split(tag2.Name, "-")[0] {
				patches = backward
			}
			if err := g.renderFilesChangesForTags(tag1, tag2, patches); err != nil {
				return fmt.Errorf("render files for tags %s -> %s: %w", tag1.Name, tag2.Name, err)
			}
		}
	}
	return nil
}

func (g *generator) renderFilesChangesForTags(tag1, tag2 tag, patches []patch) error {
	if err := os.MkdirAll(filepath.Join("output", "files", tag1.Name), 0755); err != nil {
		return fmt.Errorf("create output/files/%s: %w", tag1.Name, err)
	}

	f, err := os.Create(filepath.Join("output", "files", tag1.Name, tag2.Name+".html"))
	if err != nil {
		return fmt.Errorf("create output/files/%s/%s.html: %w", tag1.Name, tag2.Name, err)
	}

	if tag1 == tag2 {
		if err := g.tmpl.ExecuteTemplate(f, "files.gohtml", struct {
			Tag1    string
			Tag2    string
			Changes []file
		}{
			Tag1:    tag1.Name,
			Tag2:    tag2.Name,
			Changes: nil,
		}); err != nil {
			return fmt.Errorf("execute template: %w", err)
		}
		return nil
	}

	// log.Printf("Collecting changes between %s and %s", tag1.Name, tag2.Name)

	// collect all changes between the two tags
	changes := map[string]file{}

	collecting := false
	for _, patch := range patches {
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
					if _, ok := changes[from.Path()]; ok {
						delete(changes, from.Path())
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

		if patch.to == tag1.Name || patch.to == tag2.Name {
			// log.Printf("Collecting toggle by tag %s", patch.to)
			collecting = !collecting
			if !collecting {
				break
			}
		}
	}

	if collecting {
		// did not find the second tag, return error
		return fmt.Errorf("did not find second tag (%q or %q", tag1, tag2)
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

	if err := g.tmpl.ExecuteTemplate(f, "files.gohtml", struct {
		Tag1    string
		Tag2    string
		Changes []file
	}{
		Tag1:    tag1.Name,
		Tag2:    tag2.Name,
		Changes: changesList,
	}); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

func (g *generator) copyStaticFiles() error {
	// copy all files from "static" to "output" directory
	if err := filepath.Walk("static", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk: %w", err)
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel("static", path)
		if err != nil {
			return fmt.Errorf("get relative path: %w", err)
		}

		outputPath := filepath.Join("output", relPath)

		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}

		if err := copyFile(path, outputPath); err != nil {
			return fmt.Errorf("copy file: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
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

type patch struct {
	from     string
	to       string
	fromHash string
	toHash   string
	changes  []diff.FilePatch
}

func getDiff(r *git.Repository, tags []tag) ([]patch, []patch, error) {
	sort.Slice(tags, func(i, j int) bool {
		// tag has a format "2.10-v3877"
		// we want to sort by "v3877" part acsending
		return strings.Split(tags[i].Name, "-")[1] < strings.Split(tags[j].Name, "-")[1]
	})

	forward, err := getPatches(r, tags)
	if err != nil {
		return nil, nil, fmt.Errorf("get forward patches: %w", err)
	}

	sort.Slice(tags, func(i, j int) bool {
		// tag has a format "2.10-v3877"
		// we want to sort by "v3877" part descending
		return strings.Split(tags[i].Name, "-")[1] > strings.Split(tags[j].Name, "-")[1]
	})

	backward, err := getPatches(r, tags)
	if err != nil {
		return nil, nil, fmt.Errorf("get forward patches: %w", err)
	}

	return forward, backward, nil
}

func getPatches(r *git.Repository, tags []tag) ([]patch, error) {
	var result []patch

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
