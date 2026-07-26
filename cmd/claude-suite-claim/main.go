// Command claude-suite-claim lets an AI agent in any IDE take part in an
// adjudication session.
//
// It exists because agents cannot speak websockets but can all run a shell
// command, and because a conclusion that arrives without a way to check it is
// not worth much. A claim names a check or is recorded as an opinion that cannot
// block anything — it never supplies a command of its own.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"claude_suite/backend/claims"
)

type claimFlag struct {
	subjects   []string
	assertions []string
	falsifiers []string
}

func (c *claimFlag) addSubject(v string) error   { c.subjects = append(c.subjects, v); return nil }
func (c *claimFlag) addAssertion(v string) error { c.assertions = append(c.assertions, v); return nil }
func (c *claimFlag) addFalsifier(v string) error { c.falsifiers = append(c.falsifiers, v); return nil }

type repeated struct{ add func(string) error }

func (r repeated) String() string     { return "" }
func (r repeated) Set(v string) error { return r.add(v) }

func main() {
	var (
		host     = flag.String("host", "", "session host, e.g. ws://10.0.0.4:9111")
		session  = flag.String("session", "", "session id")
		token    = flag.String("token", "", "join token")
		author   = flag.String("author", "", "who is claiming (default: user@host/claude-code)")
		provider = flag.String("provider", "", "model provider, e.g. claude or gemini")
		outDir   = flag.String("out", ".claude-suite", "where to write verdict.json")
		wait     = flag.Duration("wait", 30*time.Minute, "how long to wait for the outcome")
		list     = flag.Bool("checks", false, "list the checks a claim may name, and exit")
		wsPath   = flag.String("workspace", ".", "workspace root, for --checks")
	)

	var c claimFlag
	flag.Var(repeated{c.addSubject}, "subject", "what the claim is about (repeatable)")
	flag.Var(repeated{c.addAssertion}, "assert", "the defect, in one sentence (repeatable)")
	flag.Var(repeated{c.addFalsifier}, "falsify", "catalogue check that passes if the claim is wrong; empty for an opinion (repeatable)")
	flag.Parse()

	if *list {
		listChecks(*wsPath)
		return
	}

	// An empty --author used to travel all the way to the host and land in the
	// log as an empty string; the app's copy-paste command filled it with
	// "YOU/your-agent", which nobody edited. Naming the machine by default means
	// the shared command line is correct as handed over.
	who := *author
	if who == "" {
		who = defaultAuthor(*provider)
	}

	if err := run(*host, *session, *token, who, *provider, *outDir, *wait, c); err != nil {
		fmt.Fprintln(os.Stderr, "claude-suite-claim:", err)
		os.Exit(1)
	}
}

func listChecks(workspace string) {
	cat, err := claims.CatalogueFor(workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-suite-claim:", err)
		os.Exit(1)
	}
	if len(cat.Checks) == 0 {
		fmt.Printf("Nothing to check against in %s.\n\n"+
			"Checks come from the project's own tooling: a go.mod, or a package.json\n"+
			"with a test, lint, check or build script. %s can add more.\n"+
			"With none, every claim is an opinion and nothing can block.\n",
			workspace, claims.CatalogueFile)
		return
	}
	fmt.Println("Checks a claim may name. Most are discovered from the project's own")
	fmt.Printf("tooling; %s can add or override entries.\n", claims.CatalogueFile)
	fmt.Println("A falsifier PASSES when the claim is wrong, so a failing check confirms the defect.")
	fmt.Println()
	for _, check := range cat.Checks {
		fmt.Printf("  %-24s %s\n", check.Name, check.Description)
	}
}

func run(host, session, token, author, provider, outDir string, wait time.Duration, c claimFlag) error {
	if len(c.subjects) != len(c.assertions) {
		return fmt.Errorf("got %d --subject and %d --assert; they pair up one to one",
			len(c.subjects), len(c.assertions))
	}
	if len(c.falsifiers) > len(c.subjects) {
		return fmt.Errorf("more --falsify than --subject")
	}
	// Missing falsifiers are opinions, not errors: an agent that cannot name a
	// check should still be able to say what it thinks, it just cannot block.
	for len(c.falsifiers) < len(c.subjects) {
		c.falsifiers = append(c.falsifiers, "")
	}
	if len(c.subjects) == 0 {
		return fmt.Errorf("nothing to submit: pass --subject and --assert")
	}

	client := &claims.Client{
		HostURL: strings.TrimRight(host, "/"), SessionID: session, Token: token,
		Author: author, Provider: provider, OutDir: outDir,
	}
	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Close()

	opinions := 0
	for i := range c.subjects {
		if strings.TrimSpace(c.falsifiers[i]) == "" {
			opinions++
		}
		if err := client.Submit(c.subjects[i], c.assertions[i], c.falsifiers[i]); err != nil {
			return err
		}
	}
	fmt.Printf("Submitted %d claim(s); %d without a check and so cannot block.\n",
		len(c.subjects), opinions)

	if err := client.Done(); err != nil {
		return err
	}
	fmt.Println("Waiting for the other agents and for the checks to run...")

	outcome, err := client.Await(wait)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Print(outcome.Summary())
	fmt.Printf("\nWritten to %s/session-%s/\n", outDir, outcome.SessionID)

	// A non-zero exit lets a caller gate on this without parsing the summary.
	if len(outcome.Blocking) > 0 {
		os.Exit(2)
	}
	return nil
}
