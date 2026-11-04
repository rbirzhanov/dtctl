package cmd

import (
	_ "embed"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/config"
)

// serviceSurface is the command surface the embedding service permits, one
// command path per line, sorted. It is the service's own decision (its
// allowlist) and is deliberately kept apart from the
// profile definition in sdk/session/profile_service.go: the test compares
// the two, so a change to either one alone fails the build.
//
//go:embed testdata/service_surface.txt
var serviceSurface string

func serviceProfile(t *testing.T) *config.Profile {
	t.Helper()
	t.Setenv(config.ProfileEnvVar, "service")
	p, err := (&config.Config{}).ResolveProfile()
	if err != nil || p == nil {
		t.Fatalf(`built-in profile "service" does not resolve: %v`, err)
	}
	return p
}

func expectedServicePaths() map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(serviceSurface, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out[line] = true
		}
	}
	return out
}

// TestServiceProfileMatchesTheService: the profile resolves, against dtctl's
// real command tree, to exactly the service's surface (plus the always-available
// `commands`/`help` core). Both directions are checked:
//
//   - a command the profile allows but the service did not decide on (a dtctl
//     upgrade brought it, or an entry's prefix reaches below what was listed) is
//     a widening, and fails;
//   - a path the service lists that no longer names a command (renamed or
//     removed in dtctl) fails, so the drift is found at build time.
func TestServiceProfileMatchesTheService(t *testing.T) {
	p := serviceProfile(t)
	expected := expectedServicePaths()

	exists := map[string]bool{}
	var widened []string
	walkCommands(rootCmd, func(c *cobra.Command) {
		if c == rootCmd {
			return
		}
		path := commandPathRelative(c, rootCmd)
		exists[path] = true
		always := path == "commands" || strings.HasPrefix(path, "commands ") || path == "help"
		if p.Allows(path) && !expected[path] && !always {
			widened = append(widened, path)
		}
		if !p.Allows(path) && expected[path] && !emptyGroup(c, expected) {
			t.Errorf("service permits %q but the profile masks it", path)
		}
	})

	var missing []string
	for path := range expected {
		if !exists[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(widened)
	sort.Strings(missing)
	if len(widened) > 0 {
		t.Errorf("profile allows commands the service has not decided on (add them to testdata/service_surface.txt, or narrow the profile):\n  %s", strings.Join(widened, "\n  "))
	}
	if len(missing) > 0 {
		t.Errorf("service lists paths that name no dtctl command (renamed or removed?):\n  %s", strings.Join(missing, "\n  "))
	}
}

// emptyGroup reports whether c is a bare group (it only routes to subcommands) with no command
// beneath it that the service permits. The service lists such a group as an
// exact path; a prefix-matching profile cannot allow it without admitting its
// children, and masking it costs nothing.
func emptyGroup(c *cobra.Command, expected map[string]bool) bool {
	if len(c.Commands()) == 0 {
		return false // a leaf is a command, not a group
	}
	empty := true
	walkCommands(c, func(sub *cobra.Command) {
		if sub != c && expected[commandPathRelative(sub, rootCmd)] {
			empty = false
		}
	})
	return empty
}

// TestServiceProfileRefusesHostLocalCommands: the commands that manage
// host-local state stay masked.
func TestServiceProfileRefusesHostLocalCommands(t *testing.T) {
	p := serviceProfile(t)
	for _, path := range []string{"auth login", "config", "ctx", "doctor", "edit", "plugin", "serve", "skills", "completion", "alias", "inspect", "account"} {
		if p.Allows(path) {
			t.Errorf("profile must not allow %q", path)
		}
	}
}
