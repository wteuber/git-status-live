# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `git-live`, a terminal UI that shows `git status` live, refreshing every 500ms by default (`-i` to change) and redrawing only when something changes.
- List view that groups entries like `git status`: changes to be committed, unmerged paths, changes not staged for commit, and untracked files.
- Tree view in the style of [git-status-tree](https://github.com/wteuber/git-status-tree): blue directories, green for fully staged files, red for anything unstaged, annotations such as `(M)`, `(A+)`, `(M+M)` and `(?)`, and renames shown as `old -> new (R+)` at their original location.
- Keys: `q`/`Ctrl-C` quit, `t`/`Tab` toggle view, `↑`/`↓`/`j`/`k`, `PgUp`/`PgDn`/`Space` and `g`/`G`/`Home`/`End` scroll, `r` refresh now.
- Header with the current view, branch and upstream, and counts of staged, unstaged, untracked and conflicting files.
- git runs with `GIT_OPTIONAL_LOCKS=0` and runs never overlap, so git-live never takes `index.lock` and doesn't pile up git processes on slow repos.
- If the directory isn't a git repository, git's error is shown and polling continues until it becomes one.
- Builds for macOS, Linux and Windows; the only dependencies are `golang.org/x/term` and `golang.org/x/sys`.
- CI on Linux, macOS and Windows with the minimum and latest Go versions.
- `-u`/`--untracked` option to show untracked files in new directories individually (`git status --untracked-files=all`) instead of as a single `dir/` entry.
