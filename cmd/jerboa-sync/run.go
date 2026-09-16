package main

// Syncing several GarageBand projects in one run: what a drag-and-drop, a
// Finder Quick Action or a double-click on the Jerboa Sync app produces.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type chosenProject struct {
	band   string // absolute path to the .band package
	origin string // what the user chose: the package itself, or a folder of them
}

// syncTargets syncs projects named explicitly and remembers each choice, so a
// plain run later picks them up again.
func syncTargets(cfg *Config, targets []string, opts mirrorOpts) int {
	var projects []chosenProject
	bad := 0
	for _, t := range targets {
		bands, err := expandTarget(t)
		if err != nil {
			fmt.Printf("skip  %s: %v\n", t, err)
			bad++
			continue
		}
		origin, _ := filepath.Abs(t)
		// Remember up front, so a run cut short by a server problem still
		// knows about everything that was chosen.
		if !opts.DryRun {
			rememberProject(origin)
		}
		for _, b := range bands {
			projects = append(projects, chosenProject{band: b, origin: origin})
		}
	}
	code := runProjects(cfg, projects, opts)
	if bad > 0 {
		return 1
	}
	return code
}

// runProjects pushes each project in turn. One failing doesn't stop the rest,
// unless it's a failure every project would hit.
func runProjects(cfg *Config, projects []chosenProject, opts mirrorOpts) int {
	seen := map[string]bool{}
	unique := projects[:0]
	for _, p := range projects {
		if !seen[p.band] {
			seen[p.band] = true
			unique = append(unique, p)
		}
	}
	projects = unique
	if len(projects) == 0 {
		fmt.Println("nothing to sync")
		return 1
	}

	counts := map[pushResult]int{}
	for i, p := range projects {
		if len(projects) > 1 {
			fmt.Printf("\n==> [%d/%d] %s\n", i+1, len(projects), filepath.Base(p.band))
		}
		r := runBandPush(cfg, p.band, opts)
		counts[r]++

		// Declining to replace another copy's backup means "not this one",
		// so don't keep offering it. A folder stays: it was chosen as a whole.
		if r == pushDeclined && p.origin == p.band {
			forgetProject(p.band)
		}
		if r == pushAbortAll {
			if left := len(projects) - i - 1; left > 0 {
				fmt.Printf("\nskipping the other %d project(s): they'd hit the same problem.\n", left)
			}
			break
		}
	}

	if len(projects) > 1 {
		var parts []string
		for _, c := range []struct {
			r     pushResult
			label string
		}{{pushOK, "synced"}, {pushFailed, "need another run"}, {pushDeclined, "left alone"}, {pushAbortAll, "stopped"}} {
			if counts[c.r] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", counts[c.r], c.label))
			}
		}
		fmt.Printf("\n== %d projects: %s ==\n", len(projects), strings.Join(parts, ", "))
	}

	if counts[pushFailed] > 0 || counts[pushAbortAll] > 0 {
		return 1
	}
	return 0
}

// runInteractive handles a run with no arguments, usually a double-click.
func runInteractive(client *http.Client, cfg *Config, opts mirrorOpts) int {
	// Standing in a folder of projects is the most specific signal.
	if here := findBands("."); len(here) == 1 {
		return syncChosen(cfg, []chosenProject{{band: here[0], origin: here[0]}}, opts)
	} else if len(here) > 1 {
		return pickAndSync(client, cfg, opts, "GarageBand projects in this folder:", asChosen(here), nil)
	}

	if len(stored.Projects) > 0 {
		var found []chosenProject
		var missing []string
		for _, origin := range stored.Projects {
			bands, err := expandTarget(origin)
			if err != nil {
				missing = append(missing, fmt.Sprintf("%s — %v", tildePath(origin), err))
				continue
			}
			for _, b := range bands {
				found = append(found, chosenProject{band: b, origin: origin})
			}
		}
		return pickAndSync(client, cfg, opts, "Projects Jerboa Sync keeps backed up:", found, missing)
	}

	var discovered []string
	for _, dir := range discoveryDirs() {
		discovered = append(discovered, findBands(dir)...)
	}
	if len(discovered) > 0 {
		return pickAndSync(client, cfg, opts, "GarageBand projects on this computer:", asChosen(discovered), nil)
	}

	fmt.Println("There are no GarageBand projects to sync yet.")
	fmt.Println()
	if runtime.GOOS == "darwin" {
		fmt.Println("To back one up, drag it onto Jerboa Sync, or right-click it in Finder")
		fmt.Println("and choose Quick Actions -> Sync to Jerboa. It's remembered after that,")
		fmt.Println("so double-clicking Jerboa Sync keeps it backed up.")
	} else {
		fmt.Println("Run jerboa-sync with a project's path to back it up; it's remembered after that.")
	}
	fmt.Println()
	if strings.EqualFold(readLine("Type p to download a project from your band instead, or press Enter to close: "), "p") {
		return runPull(client, cfg, opts)
	}
	return 0
}

// pickAndSync lists projects and lets Enter sync them all — the common case
// for a double-click — or a number pick one.
func pickAndSync(client *http.Client, cfg *Config, opts mirrorOpts, header string, projects []chosenProject, missing []string) int {
	fmt.Println(header)
	for i, p := range projects {
		fmt.Printf("  %2d) %-32s %s\n", i+1, filepath.Base(p.band), tildePath(filepath.Dir(p.band)))
	}
	if len(missing) > 0 {
		fmt.Println("\ncan't find:")
		for _, m := range missing {
			fmt.Printf("   -  %s\n", m)
		}
		fmt.Println("   (moved it? drag it onto Jerboa Sync again. Done with it? jerboa-sync forget PATH)")
	}
	if len(projects) == 0 {
		fmt.Println()
		if strings.EqualFold(readLine("Type p to download a project from your band, or press Enter to close: "), "p") {
			return runPull(client, cfg, opts)
		}
		return 1
	}

	fmt.Println()
	choice := strings.ToLower(readLine("Press Enter to sync all of them, a number for just one, p to download a project, or q to quit: "))
	switch choice {
	case "":
		return syncChosen(cfg, projects, opts)
	case "q":
		return 0
	case "p":
		return runPull(client, cfg, opts)
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(projects) {
		fmt.Printf("didn't understand %q\n", choice)
		return 1
	}
	return syncChosen(cfg, projects[n-1:n], opts)
}

// syncChosen runs picked projects and remembers them for next time.
func syncChosen(cfg *Config, projects []chosenProject, opts mirrorOpts) int {
	if !opts.DryRun {
		for _, p := range projects {
			rememberProject(p.origin)
		}
	}
	return runProjects(cfg, projects, opts)
}

func asChosen(bands []string) []chosenProject {
	out := make([]chosenProject, len(bands))
	for i, b := range bands {
		out[i] = chosenProject{band: b, origin: b}
	}
	return out
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
