package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	flags "github.com/jessevdk/go-flags"
)

//go:embed templates/*
var templates embed.FS

type config struct {
	RepoURL      string `env:"REPO_URL" long:"url" description:"URL of the repository to clone" default:"https://github.com/ilyabirman/Aegea-Comparisons"`
	RepoPath     string `env:"REPO_PATH" long:"path" description:"Path to the repository to read"`
	TemplatesDir string `env:"TEMPLATES_DIR" long:"templates" description:"Directory with templates"`
	CopyFiles    bool   `env:"COPY_FILES" long:"copy" description:"Copy files per each tag into the output directory"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	var cfg config
	if _, err := flags.Parse(&cfg); err != nil {
		if err.(*flags.Error).Type == flags.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	repo, err := getRepo(cfg.RepoURL, cfg.RepoPath)
	if err != nil {
		return fmt.Errorf("git repo: %w", err)
	}

	var tmpl *template.Template
	if cfg.TemplatesDir != "" {
		log.Printf("Processing templates from %s", cfg.TemplatesDir)
		tmpl, err = template.New("").ParseGlob(filepath.Join(cfg.TemplatesDir, "*.gohtml"))
		if err != nil {
			return fmt.Errorf("parse templates: %w", err)
		}
	} else {
		log.Printf("Processing embedded templates")
		tmpl, err = template.New("").ParseFS(templates, "templates/*.gohtml")
		if err != nil {
			return fmt.Errorf("parse templates: %w", err)
		}
	}

	g := generator{
		repo:      repo,
		tmpl:      tmpl,
		copyFiles: cfg.CopyFiles,
	}

	if err = g.Run(); err != nil {
		return fmt.Errorf("run generator: %w", err)
	}

	return nil
}

func getRepo(repoURL, repoPath string) (*git.Repository, error) {
	if repoPath != "" {
		log.Printf("Opening %s", repoPath)
		return git.PlainOpen(repoPath)
	}

	log.Printf("Cloning %s", repoURL)
	return git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: repoURL,
	})
}
