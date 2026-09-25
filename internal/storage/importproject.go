package storage

import (
	"fmt"
	"strings"
)

// ResolveImportProject decides which project an import files its contracts
// under.
//
// Without a request it is the active project, which is what makes an unflagged
// import behave exactly as it did when there was only one. With one it is the
// named project, and a name that does not exist is an error rather than a new
// project: an import is the act of filing a client's declared contracts, and
// silently creating the project a typo named would file them somewhere nothing
// is looking. The error lists what does exist, because a refusal a user cannot
// act on is only slightly better than the wrong answer.
//
// It lives here rather than in the command because there are now two channels
// that import — the CLI against the file, and the CLI against a running
// instance's API — and the second resolves the project inside the server. Two
// copies of "which project, and what to say when it is not one" is how the two
// channels come to disagree about a typo, which is the disagreement this
// function exists to make impossible.
func ResolveImportProject(store *Store, requested string) (string, error) {
	if requested == "" {
		return store.ActiveProject(), nil
	}
	if !store.ProjectExists(requested) {
		projects, _ := store.ListProjects()
		known := make([]string, 0, len(projects))
		for _, p := range projects {
			// The id is what the flag takes, so it leads. The name is how a
			// person knows the project, so it follows when it says more.
			if p.Name != "" && p.Name != p.ID {
				known = append(known, fmt.Sprintf("%s (%s)", p.ID, p.Name))
				continue
			}
			known = append(known, p.ID)
		}
		if len(known) == 0 {
			return "", fmt.Errorf("no such project: %q, and this install has no projects", requested)
		}
		return "", fmt.Errorf("no such project: %q; known projects: %s",
			requested, strings.Join(known, ", "))
	}
	return requested, nil
}
