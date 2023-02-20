# diff

[![main](https://github.com/chuhlomin/diff/actions/workflows/main.yml/badge.svg)](https://github.com/chuhlomin/diff/actions/workflows/main.yml)

A small utility to generate a static site for comparing files between tags in Git repository using [Monaco Editor](https://microsoft.github.io/monaco-editor/).
May be useful if you don't want to publish your code to GitHub.

## Usage

```bash
$ diff --help
Usage:
  diff [OPTIONS]

Application Options:
      --url=              URL of the repository to clone (default: https://github.com/ilyabirman/Aegea-Comparisons) [$REPO_URL]
      --path=             Path to the repository to read [$REPO_PATH]
      --templates=        Directory with templates [$TEMPLATES_DIR]
      --static=           Directory with static files [$STATIC_DIR]
      --copy              Copy files per each tag into the output directory [$COPY_FILES]
      --diff-base-url=    Base URL for diff links (default: ./files/) [$DIFF_BASE_URL]
      --content-base-url= Base URL for content links (default: ./content/) [$CONTENT_BASE_URL]

Help Options:
  -h, --help              Show this help message
```

When you run the command, it will read the Git repository and generate the static site into the `./output` directory.

If `--path` is not specified, app will use the repository from the directory.
Otherwise the repository will be cloned into the memory from the specified URL in the `--url` option.

If `--copy` flag is passed, app will group files by tags and copy them into the output directory.

Binary embeds static files from `static` directory and templates from `templates` directory.
They can be overridden by `--static` and `--templates` options.
Static files will be copied into the output directory.

There are two Go Templates in the `templates` directory: `index.gohtml` and `files.gohtml`.

`index.gohtml` template is used to generate the index page.
It has `Tags` variable - list of tags in the repository

`files.gohtml` template is used to generate the list of changed files from tag to tag.
It has the following variables:

* `Changes` - list of changes between tags
  * `Operation` - "A" for added, "D" for deleted, "M" for modified, "R" for renamed
  * `Name` - current file name
  * `OldName` - old file name (for renamed files and deleted files)

## Local development

Pre-requisites:

* [Go 1.19](https://go.dev/dl/) or later.
* [Caddy](https://caddyserver.com/) or any other web server.

```bash
export CONTENT_BASE_URL="https://raw.githubusercontent.com/ilyabirman/Aegea-Comparisons/"
make run

caddy run
```
