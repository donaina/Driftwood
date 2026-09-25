package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

/* The vitals strip and the alerts empty state both used to read an absence of
   measurement as good news, and they were wrong in opposite directions.

   The strip scored every captured request: the numerator was the MATCHes and
   the denominator was everything. Two of the five statuses the proxy emits are
   not verdicts — NO_BASELINE, when no contract existed to compare against, and
   BASELINE_SET, when the response is what the contract was made from — so an
   install pointed at an API and called twice before its first baseline
   rendered 0% in Fault Red, having compared nothing to anything. And the empty
   case was hard-coded to 1, so an install that had seen no traffic at all
   rendered 100%, in green. The alerts view made the same inference in prose:
   no alerts became "your frontend contract is healthy", whether or not
   anything had ever been compared.

   Neither number is checkable by anything else in the build. There is no
   frontend test runner — vitest is a devDependency with no test files, and
   `make verify` runs a type-check and a bundle, neither of which evaluates a
   line of this script. So the assertion lives here, in Go, where the gate runs
   it, parsing the shell the way shell_nav_test.go does and for the same
   reason.

   What is pinned is the shape of the rule, not the wording: the set of statuses
   that count as a verdict, that the rate is computed over exactly that set, and
   that an empty set produces no score rather than a perfect one. Editing the
   prose is free; deleting the distinction is not. */

var ratedStatusesLiteral = regexp.MustCompile(`const RATED_STATUSES = \[([^\]]*)\];`)

// rationaleStatuses are the statuses that say something about whether the
// contract held. The other two the proxy emits (NO_BASELINE, BASELINE_SET) say
// only that there was nothing to compare against, or that this response is what
// the comparison will be against.
var rationaleStatuses = []string{"BREAKING", "MATCH", "WARNING"}

func TestVitalsRateOnlyVerdicts(t *testing.T) {
	src := shellSource(t)

	m := ratedStatusesLiteral.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("the shell declares no RATED_STATUSES: the vitals strip is back to scoring every captured request, including the ones nothing was compared against")
	}

	var got []string
	for _, field := range strings.Split(m[1], ",") {
		got = append(got, strings.Trim(strings.TrimSpace(field), `'"`))
	}
	sort.Strings(got)

	want := append([]string(nil), rationaleStatuses...)
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("RATED_STATUSES = %v, want %v — the proxy's own vocabulary is NO_BASELINE, MATCH, WARNING, BREAKING, plus BASELINE_SET, and only three of those are a verdict about the contract", got, want)
	}
}

func TestVitalsScoreIsTakenOverTheRatedRequests(t *testing.T) {
	src := shellSource(t)

	// The denominator is the whole point. `matchCount / trafficHistory.length`
	// is what made two never-compared requests render as 0%.
	if !strings.Contains(src, "trafficHistory.filter(t => RATED_STATUSES.includes(t.contract_status))") {
		t.Error("the vitals strip does not derive its rated set from RATED_STATUSES")
	}
	if !strings.Contains(src, "matchCount / rated.length") {
		t.Error("the contract-health rate is not taken over the rated requests")
	}
}

func TestVitalsShowNoScoreWhenNothingHasBeenRated(t *testing.T) {
	src := shellSource(t)

	if !strings.Contains(src, "const health = rated.length ? matchCount / rated.length : null;") {
		t.Error("an empty rated set does not produce null: nothing measured is being scored as if it were")
	}
	// The placeholder the strip is rendered with before any traffic arrives, so
	// a viewer sees the same thing at first paint and after an empty update.
	if !strings.Contains(src, `id="contract-health">-<`) {
		t.Error(`the vitals strip is not rendered with "-" as its initial value`)
	}
	if !strings.Contains(src, `health === null ? '-'`) {
		t.Error("an unmeasured contract-health is rendered as a number")
	}
	// The ring is read at a glance, and a stale arc left over from the last
	// update is a score for a state the strip has just said it cannot score.
	if !strings.Contains(src, "(health === null ? 0 : health * 360)") {
		t.Error("an unmeasured contract ring keeps whatever arc the last update drew")
	}
}

func TestAlertsEmptyStateDistinguishesNothingBrokenFromNothingCompared(t *testing.T) {
	src := shellSource(t)

	i := strings.Index(src, "function renderAlerts()")
	if i < 0 {
		t.Fatal("renderAlerts is gone; this test needs rewriting, not deleting")
	}
	body := src[i:]
	if end := strings.Index(body, "function renderAlerts"); end > 0 {
		body = body[:end]
	}
	if len(body) > 4000 {
		body = body[:4000]
	}

	if !strings.Contains(body, "RATED_STATUSES.includes") {
		t.Error("the alerts empty state claims the contract is healthy without checking whether anything has been compared against one")
	}
}

// A guard on the guard. The three assertions above are string matches, so they
// pass trivially if the shell they are reading is not the shell that ships.
func TestShellSourceIsTheServedPage(t *testing.T) {
	path := filepath.Join("..", "web", "index.html")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the shell the vitals tests parse is not there: %v", err)
	}
	if src := shellSource(t); !strings.Contains(src, "updateVitals") {
		t.Error("the file at web/index.html does not contain the vitals strip this file tests")
	}
}
