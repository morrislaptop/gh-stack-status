package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/morrislaptop/gh-stack-status/internal/gitcmd"
	"github.com/morrislaptop/gh-stack-status/internal/github"
	"github.com/morrislaptop/gh-stack-status/internal/render"
	"github.com/morrislaptop/gh-stack-status/internal/resolve"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("stack-status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, `Show CI checks and reviewer status for each pull request in a GitHub stack.

USAGE
  gh stack-status [<stack> | <pr> | <url> | <branch>] [flags]

The stack is the GitHub stacked-PR stack for the current branch's pull request,
or for the stack / PR / branch you pass. Local-only stacks that have not been
submitted to GitHub are not shown.

FLAGS
  -s, --short   Compact output (PR number, checks, reviews)
      --json    Output stack status as JSON
  -h, --help    Show help
`)
	}

	var short, jsonOut, help bool
	fs.BoolVar(&short, "s", false, "Compact output")
	fs.BoolVar(&short, "short", false, "Compact output")
	fs.BoolVar(&jsonOut, "json", false, "Output stack status as JSON")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.BoolVar(&help, "help", false, "Show help")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if help {
		fs.Usage()
		return nil
	}

	var target string
	switch fs.NArg() {
	case 0:
	case 1:
		target = fs.Arg(0)
	default:
		return fmt.Errorf("too many arguments")
	}

	repo, err := repository.Current()
	if err != nil {
		return fmt.Errorf("could not determine current repository: %w", err)
	}

	branch, err := gitcmd.CurrentBranch()
	if err != nil && target == "" {
		return err
	}

	client, err := github.NewClient()
	if err != nil {
		return err
	}

	stack, err := resolve.Load(client, resolve.Repo{Owner: repo.Owner, Name: repo.Name}, branch, target)
	if err != nil {
		return err
	}

	pal := render.Palette(render.PlainPalette{})
	if !jsonOut {
		pal = render.PaletteFromEnv()
	}

	return render.Write(stdout, stack, pal, render.Options{JSON: jsonOut, Short: short})
}
