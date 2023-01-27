package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type generator struct {
	repo           *git.Repository
	tmpl           *template.Template
	staticDir      string
	copyFiles      bool
	diffBaseURL    string
	contentBaseURL string

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

	if err := g.renderFilesChanges(tags); err != nil {
		return fmt.Errorf("render files: %w", err)
	}

	if g.copyFiles {
		log.Printf("Pulling files")
		if err := g.pullFiles(tags); err != nil {
			return fmt.Errorf("pull files: %w", err)
		}
	}

	if g.staticDir != "" {
		log.Printf("Copying static files from %s", g.staticDir)
		if err := g.copyStaticFiles(); err != nil {
			return fmt.Errorf("copy static files: %w", err)
		}
	} else {
		log.Printf("Copying embedded static files")
		if err := g.copyEmbeddedStaticFiles(); err != nil {
			return fmt.Errorf("copy embedded static files: %w", err)
		}
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
		Tags           []tag
		DiffBaseURL    string
		ContentBaseURL string
	}{
		Tags:           tags,
		DiffBaseURL:    g.diffBaseURL,
		ContentBaseURL: g.contentBaseURL,
	}); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

type file struct {
	Name      string
	OldName   string
	Operation string // A, D, M, R
}

func (f file) Less(other file) bool {
	return f.Name < other.Name
}

func (g *generator) renderFilesChanges(tags []tag) error {
	// get all combinations of tags
	for _, tag1 := range tags {
		for _, tag2 := range tags {
			log.Printf("Rendering files changes between %s and %s", tag1.Name, tag2.Name)
			if err := g.renderFilesChangesBetweenTags(tag1, tag2); err != nil {
				return fmt.Errorf("render files for tags %s -> %s: %w", tag1.Name, tag2.Name, err)
			}
			if err := g.renderFilesChangesBetweenTags(tag2, tag1); err != nil {
				return fmt.Errorf("render files for tags %s -> %s: %w", tag1.Name, tag2.Name, err)
			}
		}
	}
	return nil
}

func (g *generator) renderFilesChangesBetweenTags(tag1, tag2 tag) error {
	changes, err := g.diff(tag1, tag2)
	if err != nil {
		return fmt.Errorf("collect changes: %w", err)
	}

	if err := os.MkdirAll(filepath.Join("output", "files", tag1.Name), 0755); err != nil {
		return fmt.Errorf("create output/files/%s: %w", tag1.Name, err)
	}

	f, err := os.Create(filepath.Join("output", "files", tag1.Name, tag2.Name+".html"))
	if err != nil {
		return fmt.Errorf("create output/files/%s/%s.html: %w", tag1.Name, tag2.Name, err)
	}

	if err := g.tmpl.ExecuteTemplate(f, "files.gohtml", struct {
		Tag1    string
		Tag2    string
		Changes []file
	}{
		Tag1:    tag1.Name,
		Tag2:    tag2.Name,
		Changes: changes,
	}); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

func (g *generator) pullFiles(tags []tag) error {
	// get all files in the tag
	for _, tag := range tags {
		commit, err := g.repo.CommitObject(tag.Hash)
		if err != nil {
			return fmt.Errorf("get commit for tag %q: %w", tag, err)
		}

		tree, err := commit.Tree()
		if err != nil {
			return fmt.Errorf("get tree: %w", err)
		}

		err = tree.Files().ForEach(func(file *object.File) error {
			content, err := file.Contents()
			if err != nil {
				return fmt.Errorf("get file content: %w", err)
			}

			filePath := filepath.Join("output", "content", tag.Name, file.Name)

			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}

			f, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}

			if _, err := f.WriteString(content); err != nil {
				return fmt.Errorf("write file: %w", err)
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("iterate files: %w", err)
		}
	}

	return nil
}

func (g *generator) copyStaticFiles() error {
	// copy all files from "static" to "output" directory
	if err := filepath.Walk(g.staticDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk: %w", err)
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(g.staticDir, path)
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

//go:embed static
var static embed.FS

func (g *generator) copyEmbeddedStaticFiles() error {
	// copy all files from static embed.FS to "output" directory
	if err := fs.WalkDir(static, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk: %w", err)
		}

		if d.IsDir() {
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

		f, err := static.Open(path)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}

		out, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}

		if _, err := io.Copy(out, f); err != nil {
			return fmt.Errorf("copy: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	return nil
}

func (g *generator) diff(tag1, tag2 tag) ([]file, error) {
	commit1, err := g.repo.CommitObject(tag1.Hash)
	if err != nil {
		return nil, fmt.Errorf("get commit for tag %q: %w", tag1.Name, err)
	}

	commit2, err := g.repo.CommitObject(tag2.Hash)
	if err != nil {
		return nil, fmt.Errorf("get commit for tag %q: %w", tag2.Name, err)
	}

	p, err := commit1.Patch(commit2)
	if err != nil {
		return nil, fmt.Errorf("get patch: %w", err)
	}

	patches := p.FilePatches()
	changes := make([]file, 0, len(patches))
	for _, patch := range patches {
		if patch.IsBinary() {
			continue
		}

		from, to := patch.Files()

		var toPath, fromPath string
		if to != nil {
			toPath = to.Path()
		}
		if from != nil {
			fromPath = from.Path()
		}

		if toPath == fromPath {
			if !hasChanges(patch) {
				continue
			}
		}

		changes = append(changes, file{
			Name:    toPath,
			OldName: fromPath,
			Operation: func(to, from string) string {
				if from == "" {
					return "A"
				}

				if to == "" {
					return "D"
				}

				if from != to {
					return "R"
				}

				return "M"
			}(toPath, fromPath),
		})
	}

	return changes, nil
}

func hasChanges(patch diff.FilePatch) bool {
	for _, chunk := range patch.Chunks() {
		if chunk.Type() != diff.Equal {
			return true
		}
	}
	return false
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
	Name    string
	Hash    plumbing.Hash
	version int
}

func (t tag) Version() int {
	if t.version != 0 {
		return t.version
	}

	// tag has a format "2.10-v3877"
	// we want to return "3877" part
	parts := strings.Split(t.Name, "v")
	if len(parts) != 2 {
		log.Printf("tag %q has unexpected format", t.Name)
		return 0
	}

	v, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Printf("tag %q has unexpected format: %v", t.Name, err)
		return 0
	}

	t.version = v
	return v
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
		return tags[i].Version() > tags[j].Version()
	})

	return tags, nil
}

type patch struct {
	from     string
	to       string
	fromHash string
	toHash   string
	changes  []file
}
