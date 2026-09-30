# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Worktree list: `w` shows every worktree of the repository with its branch (or detached commit), a colored summary of its changes and its path, refreshed every interval while it is open. The list appears right away, and each summary shows `loading…` until git status in that worktree finishes. The watched worktree is marked with `*`. Locked worktrees are marked `locked`; prunable worktrees and bare repositories are listed but can't be selected.
- Switch worktrees: select one with `↑`/`↓`/`j`/`k`, `PgUp`/`PgDn`/`Space` or `g`/`G`/`Home`/`End` and press `Enter` to watch it instead. `w` or `Esc` go back without switching.
- Search worktrees: `/` shows only the worktrees whose name, branch or path contain every word typed, ignoring case. `Backspace` edits the search and `Esc` clears it.
- Scroll sideways with `←`/`→` or `h`/`l`, half a screen at a time, to see lines that are cut off at the right edge, e.g. long paths in the worktree list. The footer shows the first visible column.
- `u` toggles `-u`/`--untracked` while git live is running, in the list, the tree and the worktree list. The header shows `-u` while it is on.
- `--help` prints the usage like `-h`, and the usage now links to the source at https://github.com/wteuber/git-status-live.

### Changed
- The header shows the name of the watched worktree (its directory) between the view and the branch.

## [0.2.0] - 2026-09-29

### Added
- `--view list|tree` option and `live.view` git config setting to choose the view `git live` starts in. `--view` takes precedence over `live.view`; without either, it starts in the list view as before.

## [0.1.0] - 2026-09-29

First release.

### Added
- `git-live`, a terminal UI that shows `git status` live, refreshing every 500ms by default (`-i` to change) and redrawing only when something changes.
- List view that groups entries like `git status`: changes to be committed, unmerged paths, changes not staged for commit, and untracked files.
- Tree view in the style of [git-status-tree](https://github.com/wteuber/git-status-tree): blue directories, green for fully staged files, red for anything unstaged, annotations such as `(M)`, `(A+)`, `(M+M)` and `(?)`, and renames shown as `old -> new (R+)` at their original location.
- `-u`/`--untracked` option to show untracked files in new directories individually (`git status --untracked-files=all`) instead of as a single `dir/` entry.
- `-h` prints the usage to stdout.
- Keys: `q`/`Ctrl-C` quit, `t`/`Tab` toggle view, `↑`/`↓`/`j`/`k`, `PgUp`/`PgDn`/`Space` and `g`/`G`/`Home`/`End` scroll, `r` refresh now.
- Header with the current view, branch and upstream, and counts of staged, unstaged, untracked and conflicting files; a footer with the keys and, when the content doesn't fit, the visible lines.
- git runs with `GIT_OPTIONAL_LOCKS=0` and runs never overlap, so git-live never takes `index.lock` and doesn't pile up git processes on slow repos.
- If the directory isn't a git repository, git's error is shown and polling continues until it becomes one.
- Builds for macOS, Linux and Windows; the only dependencies are `golang.org/x/term` and `golang.org/x/sys`.
- Installation with a single command that also sets up the `git live` alias.
- CI on Linux, macOS and Windows with the minimum and latest Go versions, and a minimum test coverage of 89%.

[Unreleased]: https://github.com/wteuber/git-status-live/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/wteuber/git-status-live/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/wteuber/git-status-live/releases/tag/v0.1.0
