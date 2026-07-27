// Command agent-center-claim lets an AI agent in any IDE take part in an
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

	"agent_center/backend/claims"
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
		outDir   = flag.String("out", ".agent-center", "where to write verdict.json")
		wait     = flag.Duration("wait", 30*time.Minute, "how long to wait for the outcome")
		list     = flag.Bool("checks", false, "list the checks a claim may name, and exit")
		wsPath   = flag.String("workspace", ".", "workspace root, for --checks")
		say      = flag.String("say", "", "post a message to the session and exit (discussion, not a claim)")
		ping     = flag.Bool("ping", false, "check the session can be reached, then exit")
		listen   = flag.Duration("listen", 0, "follow the session's chat for this long (e.g. 10m), printing each message as one JSON line, then exit")
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

	// Both are cheap ways to answer "does this work at all" before an agent
	// discovers otherwise mid-review.
	if *ping {
		if err := pingSession(*host, *session, *token, who, *provider); err != nil {
			fmt.Fprintln(os.Stderr, "agent-center-claim:", err)
			os.Exit(1)
		}
		fmt.Println("ok: connected to", *host, "session", *session)
		return
	}

	if *say != "" {
		if err := sayInSession(*host, *session, *token, who, *provider, *say); err != nil {
			fmt.Fprintln(os.Stderr, "agent-center-claim:", err)
			os.Exit(1)
		}
		fmt.Println("sent")
		return
	}

	if *listen > 0 {
		if err := listenToSession(*host, *session, *token, who, *provider, *listen); err != nil {
			fmt.Fprintln(os.Stderr, "agent-center-claim:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(*host, *session, *token, who, *provider, *outDir, *wait, c); err != nil {
		fmt.Fprintln(os.Stderr, "agent-center-claim:", err)
		os.Exit(1)
	}
}

func listChecks(workspace string) {
	cat, err := claims.CatalogueFor(workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-center-claim:", err)
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

// watchClient attaches as an observer, which is all --ping, --say and --listen
// need. Watching rather than joining matters: joining is permanent and
// single-shot per author, so a ping before a claim run — or a say after one —
// used to fail with "has already joined" under the very author name the
// commands share by default.
func watchClient(host, session, token, author, provider string) (*claims.Client, error) {
	client := &claims.Client{
		HostURL: strings.TrimRight(host, "/"), SessionID: session, Token: token,
		Author: author, Provider: provider,
	}
	if err := client.Watch(); err != nil {
		return nil, err
	}
	return client, nil
}

// pingSession exists because the failure it detects — wrong host, expired token,
// firewall — used to surface only when an agent tried to submit, halfway through
// a review, with an error that named none of those causes.
func pingSession(host, session, token, author, provider string) error {
	client, err := watchClient(host, session, token, author, provider)
	if err != nil {
		return err
	}
	client.Close()
	return nil
}

func sayInSession(host, session, token, author, provider, text string) error {
	client, err := watchClient(host, session, token, author, provider)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Say(text)
}

// listenToSession follows the chat, one JSON line per message on stdout. An
// agent runs this in the background and reads lines as the others speak;
// paired with --say it is the conversation loop for tools that cannot hold an
// MCP connection.
func listenToSession(host, session, token, author, provider string, howLong time.Duration) error {
	client, err := watchClient(host, session, token, author, provider)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Listen(os.Stdout, howLong)
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
