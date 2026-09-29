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

See [CHANGELOG.md](CHANGELOG.md) for a list of changes between versions.
___
## Features

- **Live:** polls `git status` every 500ms (configurable) and redraws only when
  something changes, so the screen stays perfectly still while you work.
- **Two views:** a grouped list exactly like `git status`, and a colored file
  tree like `git tree`. Toggle with `t` or `Tab`.
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
| `↑` `↓`, `k` `j`        | Scroll one line               |
| `PgUp` `PgDn`, `Space`  | Scroll one page               |
| `g` `G`, `Home` `End`   | Jump to top / bottom          |
| `r`                     | Refresh now                   |

The header shows the current view, the branch with its upstream and
ahead/behind counts, and how many files are staged, unstaged, untracked or in
conflict. When the list is longer than the screen, the footer shows which lines
are visible.

## Options

```
git live [-i interval] [-u] [path]

-i duration       Refresh interval, e.g. 250ms or 2s (default 500ms)
-u, --untracked   Show untracked files in new directories
-h                Show help message
path              Repository to watch (default: current directory)
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

**Note:** Due to how git handles aliases, `git live --help` shows the alias
expansion instead of the help message. Use `git live -h` to see the help
message.

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
 LIST  │  main...origin/main  │  3 staged, 2 unstaged, 1 untracked
Changes to be committed:
        deleted:    .gitignore
        new file:   NOTES.md
        renamed:    cmd/git-live/render.go -> cmd/git-live/ansi.go

Changes not staged for commit:
        modified:   NOTES.md
        modified:   README.md

Untracked files:
        cmd/git-live/scratch.txt

 q quit  t/Tab toggle view  ↑↓/jk scroll  r refresh
```

Press `t` to switch to the tree view:

```
 TREE  │  main...origin/main  │  3 staged, 2 unstaged, 1 untracked
.
├── cmd
│   └── git-live
│       ├── render.go -> ansi.go (R+)
│       └── scratch.txt (?)
├── .gitignore (D+)
├── NOTES.md (A+M)
└── README.md (M)

 q quit  t/Tab toggle view  ↑↓/jk scroll  r refresh
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
  `index.lock`)
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
its keys, git results and output as parameters, and one test drives the whole
app, from `git status` polling to key presses, against a real repository.

## License

Copyright (c) 2026 Wolfgang Teuber. You may use git-status-live under the terms
of either the MIT License or the GNU General Public License (GPL) Version 2. See
[LICENSE](LICENSE), [MIT-LICENSE](MIT-LICENSE) and [GPL-LICENSE](GPL-LICENSE).
