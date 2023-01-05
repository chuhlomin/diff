package main

import (
	"fmt"
	"html/template"
	"log"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	flags "github.com/jessevdk/go-flags"
)

type config struct {
	Repo string `env:"REPO_URL" long:"repo" description:"URL of the repository to clone" default:"https://github.com/ilyabirman/Aegea-Comparisons"`
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

	log.Printf("Processing templates")
	tmpl, err := template.New("").ParseGlob("templates/*.gohtml")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	g := generator{
		repo: repo,
		tmpl: tmpl,
	}

	if err = g.Run(); err != nil {
		return fmt.Errorf("run generator: %w", err)
	}

	return nil
}
