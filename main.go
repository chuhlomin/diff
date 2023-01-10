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
	Repo         string `env:"REPO_URL" long:"repo" description:"URL of the repository to clone" default:"https://github.com/ilyabirman/Aegea-Comparisons"`
	TemplatesDir string `env:"TEMPLATES_DIR" long:"templates" description:"Directory with templates"`
	StaticDir    string `env:"STATIC_DIR" long:"static" description:"Directory with static files"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	var cfg config
	if _, err := flags.Parse(&cfg); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	log.Printf("Cloning %s", cfg.Repo)
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: cfg.Repo,
	})
	if err != nil {
		return fmt.Errorf("git clone: %w", err)
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
		staticDir: cfg.StaticDir,
	}

	if err = g.Run(); err != nil {
		return fmt.Errorf("run generator: %w", err)
	}

	return nil
}
