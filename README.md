[![CI](https://github.com/wteuber/git-status-live/actions/workflows/ci.yml/badge.svg)](https://github.com/wteuber/git-status-live/actions/workflows/ci.yml) [![License: MIT or GPL-2.0](https://img.shields.io/badge/license-MIT%20or%20GPL--2.0-blue.svg)](LICENSE) [![Go Version](https://img.shields.io/badge/go-%3E%3D%201.26-00ADD8.svg)](https://go.dev)

# git-status-live

git-status-live (https://github.com/wteuber/git-status-live) is `htop` for your
git repository. Instead of running `git status` over and over, run `git live`
once and keep it open in a terminal next to your editor: it shows the status of
your working tree and updates within half a second of every change, the way an
IDE does.

Switch between a `git status`-style **list** and a
[git-status-tree](https://github.com/wteuber/git-status-tree)-style **tree** with
a single key. Files that are untracked (?), added (A), modified (M), deleted (D)
or renamed (R) are colored by whether they are staged (green)(+) or not (red).

Working with several [worktrees](https://git-scm.com/docs/git-worktree), e.g.
one per coding agent? Press `w` to see all of them with their changes at a
glance, search them, and switch the live view to any one of them.

See [CHANGELOG.md](CHANGELOG.md) for a list of changes between versions.
___
## Features

- **Live:** polls `git status` every 500ms (configurable) and redraws only when
  something changes, so the screen stays perfectly still while you work.
- **Two views:** a grouped list exactly like `git status`, and a colored file
  tree like `git tree`. Toggle with `t` or `Tab`, and choose the one to start
  in with `--view` or `git config live.view`.
- **Worktrees:** browse and search all worktrees of the repository, see which
  ones have changes, and switch the live view to any of them without leaving
  git live.
- **Safe to leave running:** runs git with `GIT_OPTIONAL_LOCKS=0`, so it never
  takes `index.lock` and never gets in the way of your own `git` commands.
- **Kind to big repos:** runs never overlap; on a slow repo it simply checks
  less often instead of piling up git processes.
- **Portable:** a single binary for macOS, Linux and Windows. The only
  dependencies are the Go team's `golang.org/x/term` and `golang.org/x/sys`.

## Installation

Install with [Go](https://go.dev/dl) 1.26 or newer:

```
go install github.com/wteuber/git-status-live/cmd/git-live@latest && git config --global alias.live '!exec ~/go/bin/git-live'
```

This builds a `git-live` binary into `~/go/bin` and registers it as the git
alias `live`, so `git live` works in every repository without changing your
`PATH`. Run the same command again to update.

If you set `GOPATH` or `GOBIN`, use the directory that `go install` wrote to in
the alias instead of `~/go/bin`. If that directory is already on your `PATH`,
you can skip the alias: git runs any `git-<name>` executable on the `PATH` as
`git <name>`.

## Usage

Start git-status-live in any git repository by running:

```
git live
```

<!-- Screenshot: add an image of the tree view here, e.g.
<img width="600" alt="git live tree view" src="https://github.com/user-attachments/assets/..." />
-->

#### Keys

| Key                     | Action                        |
| ----------------------- | ----------------------------- |
| `q`, `Ctrl-C`           | Quit                          |
| `t`, `Tab`              | Toggle between list and tree  |
| `w`                     | Show the [worktrees](#worktrees) |
| `u`                     | Toggle untracked files in new directories (`-u`) |
| `↑` `↓`, `k` `j`        | Scroll one line               |
| `←` `→`, `h` `l`        | Scroll sideways by half a screen |
| `PgUp` `PgDn`, `Space`  | Scroll one page               |
| `g` `G`, `Home` `End`   | Jump to top / bottom          |
| `r`                     | Refresh now                   |

The header shows the current view (with `-u` while untracked files are listed
one by one), the name of the worktree (its directory), the branch with its
upstream and ahead/behind counts, and how many files are staged, unstaged,
untracked or in conflict. When the list is longer than the screen, the footer
shows which lines are visible (`1-20/57`).

Lines that are wider than the terminal are cut off at the right edge. Scroll
sideways with `←` `→` (or `h` `l`) to see the rest; the footer then shows the
first visible column (`col 41`). The header and footer stay in place, and
switching views or worktrees starts at the left again.

## Worktrees

If you work in several [worktrees](https://git-scm.com/docs/git-worktree) of
the same repository, for example one per coding agent, press `w` to list all of
them:

```
 WORKTREES  │  4 worktrees
* shop          main                 1 unstaged             ~/src/shop
  shop-agent-1  claude/fix-checkout  2 unstaged             ~/src/shop-agent-1
  shop-agent-2  claude/add-tests     1 staged, 2 untracked  ~/src/shop-agent-2
  shop-review   (detached 4945c5b)   clean                  ~/src/shop-review

 enter switch  ↑↓/jk select  ←→/hl scroll  / search  u untracked  w/esc back  r refresh  q quit
```

Each row shows the worktree's directory name, its branch (or the commit, if
its HEAD is detached), a summary of its changes, and its path. The worktree
git live is watching is marked with `*` and selected when the list opens. The
summary is green if all changes are staged, red if anything is unstaged,
untracked or in conflict, and dimmed if the worktree is clean. The list
refreshes every interval while it is open, so you can watch several agents at
work at once.

The list shows up as soon as git has listed the worktrees. git status then runs
in all of them in parallel, and each summary shows `loading…` until its own
status is in, so one slow worktree doesn't hold up the others. When the list
refreshes, the summaries keep their last status until the new one arrives.

Select a worktree and press `Enter` to switch the live view to it. The header
then shows its name:

```
 LIST  │  shop-agent-2  │  claude/add-tests  │  1 staged, 2 untracked
Changes to be committed:
        new file:   test/cart_test.rb

Untracked files:
        notes.md
        test/checkout_test.rb
```

#### Search

Press `/` and type to show only the worktrees whose name, branch or path
contain what you type. Separate words with spaces to narrow it down further:
every word has to match, in any order and ignoring case (`agent tests` finds
`shop-agent-2` on `claude/add-tests`).

```
 WORKTREES  │  2 of 4 worktrees  │  /agent▏
  shop-agent-1  claude/fix-checkout  2 unstaged             ~/src/shop-agent-1
  shop-agent-2  claude/add-tests     1 staged, 2 untracked  ~/src/shop-agent-2
```

While you search, the keys type text, except for the ones below.

#### Keys in the worktree list

| Key                     | Action                                          |
| ----------------------- | ----------------------------------------------- |
| `Enter`                 | Watch the selected worktree                     |
| `↑` `↓`, `k` `j`        | Select the previous / next worktree             |
| `←` `→`, `h` `l`        | Scroll sideways, e.g. to see long paths         |
| `PgUp` `PgDn`, `Space`  | Move the selection by one page                  |
| `g` `G`, `Home` `End`   | Select the first / last worktree                |
| `/`                     | Search                                          |
| `u`                     | Toggle untracked files in new directories (`-u`) |
| `w`, `Esc`              | Back to the list or tree, without switching     |
| `r`                     | Refresh now                                     |
| `q`, `Ctrl-C`           | Quit                                            |

While searching:

| Key                     | Action                                          |
| ----------------------- | ----------------------------------------------- |
| Any character           | Add it to the search                            |
| `Backspace`             | Delete the last character; on an empty search, stop searching |
| `↑` `↓`, `PgUp` `PgDn`, `Home` `End` | Move the selection among the matches |
| `←` `→`                 | Scroll sideways                                 |
| `Enter`                 | Watch the selected worktree                     |
| `Esc`                   | Clear the search and show all worktrees again   |
| `Ctrl-C`                | Quit                                            |

#### Special worktrees

- **Locked** worktrees (`git worktree lock`) show `locked` after their summary.
  You can still watch them.
- **Prunable** worktrees, whose directory was deleted without
  `git worktree remove`, show `missing, prunable` and can't be selected. Run
  `git worktree prune` to remove them from the list.
- A **bare** repository shows `(bare)` and `no working tree`, and can't be
  selected either, since there are no files to show.

git live keeps the view (list or tree) when you switch worktrees. Starting git
live inside any worktree watches that worktree, as always; the path argument
works the same way, e.g. `git live ~/src/shop-agent-1`.

## Options

```
git live [-i interval] [-u] [--view list|tree] [path]

-i duration        Refresh interval, e.g. 250ms or 2s (default 500ms)
-u, --untracked    Show untracked files in new directories (toggle with u)
--view list|tree   View to start in (default: git config live.view, or list)
-h, --help         Show help message and a link to this repository
path               Repository to watch (default: current directory)
```

By default, like `git status`, a new directory that git doesn't track yet is
shown as a single entry (`newdir/`). With `-u` or `--untracked`, every file
inside it is listed (`git status --untracked-files=all`):

```
git live                    git live -u
.                           .
├── newdir (?)              ├── newdir
└── top.txt (?)             │   ├── sub
                            │   │   └── b.txt (?)
                            │   └── a.txt (?)
                            └── top.txt (?)
```

Press `u` while git live is running to switch between the two. The header shows
`-u` while every file is listed, e.g. ` LIST -u  │  …`.

**Note:** Due to how git handles aliases, `git live --help` shows the alias
expansion instead of the help message: git turns `--help` into `git help live`
before it runs the alias. Use `git live -h`, or run the binary directly with
`~/go/bin/git-live --help`, to see the help message.

## Configuration

git live is configured with `git config`, like git itself. There are no
configuration files of its own.

#### Settings

| Setting      | Values           | Default | Command line | Effect                    |
| ------------ | ---------------- | ------- | ------------ | ------------------------- |
| `live.view`  | `list` or `tree` | `list`  | `--view`     | The view git live starts in |

For example, to always start in the tree view:

```
git config --global live.view tree
```

#### Global or per repository

- `git config --global live.view tree` sets it for all your repositories, in
  `~/.gitconfig`.
- `git config live.view tree`, run inside a repository, sets it for that
  repository only, in its `.git/config`. It takes precedence over the global
  setting, and applies to all worktrees of the repository.
- On the command line, `--view` takes precedence over both, for a single run:
  `git live --view list`.

git live reads the settings of the repository it watches, so
`git live ~/src/other-repo` uses the settings of `other-repo`.

#### Check and undo

```
git config --get live.view                        # the value git live uses here
git config --show-origin --get-regexp '^live\.'   # every live.* setting, and where it is set
git config --global --unset live.view             # back to the default
```

A value git live doesn't know stops it with an error that names the setting,
e.g. `git live: git config live.view: unknown view "grid", want list or tree`.

#### Options in the alias

The `live` alias from the [installation](#installation) is git config too. Add
options to it to use them every time, e.g. to always list untracked files one
by one and refresh every 250ms:

```
git config --global alias.live '!exec ~/go/bin/git-live -u -i 250ms'
```

Options and a path given on the command line are added after them, so
`git live --view tree ~/src/shop` still works. `u` still toggles untracked
files while git live runs.

## Try it

```
git clone https://github.com/wteuber/git-status-live.git
cd git-status-live

echo "change" >> README.md
echo "notes" > NOTES.md
git add NOTES.md
echo "more notes" >> NOTES.md
git mv cmd/git-live/render.go cmd/git-live/ansi.go
git rm -q .gitignore
echo "scratch" > cmd/git-live/scratch.txt

git live
```

```
 LIST  │  git-status-live  │  main...origin/main  │  3 staged, 2 unstaged, 1 untracked
Changes to be committed:
        deleted:    .gitignore
        new file:   NOTES.md
        renamed:    cmd/git-live/render.go -> cmd/git-live/ansi.go

Changes not staged for commit:
        modified:   NOTES.md
        modified:   README.md

Untracked files:
        cmd/git-live/scratch.txt

 q quit  t/Tab toggle view  w worktrees  u untracked  ↑↓←→/hjkl scroll  r refresh
```

Press `t` to switch to the tree view:

```
 TREE  │  git-status-live  │  main...origin/main  │  3 staged, 2 unstaged, 1 untracked
.
├── cmd
│   └── git-live
│       ├── render.go -> ansi.go (R+)
│       └── scratch.txt (?)
├── .gitignore (D+)
├── NOTES.md (A+M)
└── README.md (M)

 q quit  t/Tab toggle view  w worktrees  u untracked  ↑↓←→/hjkl scroll  r refresh
```

Leave `git live` running while you stage (`git add NOTES.md`), commit or revert
changes in another terminal, and watch the view follow along.

```
# reset repo
git reset HEAD --hard
git clean -xdf
```

## Compatibility

git-status-live supports:
* Git (https://git-scm.com): version 2.11+ (2.15+ to poll without taking
  `index.lock`, 2.31+ to mark locked and prunable worktrees)
* macOS, Linux and other Unix systems, and Windows 10+ (Windows Terminal or
  any console with virtual terminal support)

Building from source requires Go 1.26+ (https://go.dev), the minimum version of
`golang.org/x/term`.

## Uninstall

Remove the alias and the binary:

```
git config --global --unset alias.live; rm -f ~/go/bin/git-live
```

It is safe to run more than once. If you installed into a different directory
(see [Installation](#installation)), remove `git-live` from there instead. On
Windows, the binary is `git-live.exe`.

Go also keeps the downloaded source in its module cache. It is harmless, and
`go clean -modcache` removes it together with every other cached module.
___
## Development

1. Clone this repository
   * `git clone https://github.com/wteuber/git-status-live.git`
   * `cd git-status-live`
2. Run tests
   * `go test ./...` - Run all tests, including end-to-end tests that run the
     real `git` binary against a temporary repository
   * `go test -race -cover ./...` - Run them with the race detector and print
     coverage (what the CI coverage job does)
   * `go vet ./...` - Run static checks
   * `gofmt -l .` - List files that need formatting (CI requires none)
3. Run git-status-live from the repository
   * `go run ./cmd/git-live`
4. Build and install a local binary
   * `go install ./cmd/git-live`

CI runs vet, tests and a build on Linux, macOS and Windows, against both the
minimum Go version and the latest release, and fails if test coverage drops
below 89%.

Everything is tested except the terminal setup itself (raw mode, the alternate
screen, and the Windows console mode), which needs a real terminal. The code
behind it is split so that it can be tested without one: the main loop takes
its keys, git results and output as parameters, and end-to-end tests drive the
whole app, from `git status` polling to key presses, searching and switching
worktrees, against real repositories.

## License

Copyright (c) 2026 Wolfgang Teuber. You may use git-status-live under the terms
of either the MIT License or the GNU General Public License (GPL) Version 2. See
[LICENSE](LICENSE), [MIT-LICENSE](MIT-LICENSE) and [GPL-LICENSE](GPL-LICENSE).
