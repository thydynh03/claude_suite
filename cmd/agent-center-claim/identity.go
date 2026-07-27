package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// defaultAuthor names whoever is running this command, so a claim arrives
// attributed to a person and a machine rather than to the placeholder text.
//
// The desktop app hands out a ready-made command line, and the author field in it
// used to read "YOU/your-agent". Recipients were expected to edit it; most did
// not, so claims from three different people all arrived under the same name and
// the adjudication log became useless for working out who found what.
//
// Shape is "user@host/agent", e.g. "an@may-cua-an/claude-code".
func defaultAuthor(agent string) string {
	who := currentUser()
	host := shortHostname()

	switch {
	case who != "" && host != "":
		who = who + "@" + host
	case who == "":
		who = host
	}
	if who == "" {
		who = "unknown"
	}

	if agent == "" {
		agent = "agent"
	}
	return who + "/" + agent
}

// currentUser prefers the OS account name and falls back to the home directory's
// last element, which is the same thing on Windows and on most Linux setups but
// survives a user.Current() that fails in a stripped container.
func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		name := u.Username
		// Windows reports DOMAIN\user; the domain is noise in a claim.
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
		if name != "" {
			return sanitise(name)
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		if base := filepath.Base(home); base != "." && base != string(filepath.Separator) {
			return sanitise(base)
		}
	}
	return ""
}

func shortHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	// Trim the DNS suffix: "may-cua-an.local" reads worse than "may-cua-an" and
	// carries nothing extra.
	if i := strings.Index(h, "."); i > 0 {
		h = h[:i]
	}
	return sanitise(h)
}

// sanitise strips whitespace and the separators this format gives meaning to, so
// a user named "An Nguyen" cannot produce an author string that parses as
// something else.
func sanitise(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(" ", "-", "\t", "-", "/", "-", "@", "-").Replace(s)
	return s
}
