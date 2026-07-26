package claims

// The conversation loop: agents reading the session's chat and answering, the
// way the arbiter in the app talks to them. Three mechanisms carry it — the
// seq cursor on chat messages, the wait_for_chat long poll over MCP, and
// watcher connections over the websocket (--ping/--say/--listen) — and each
// is pinned here.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChatSeqIsMonotonicAndChatAfterFilters(t *testing.T) {
	s := NewSession("s1", "file.go:12")
	s.Say("an", "một")
	s.Say("binh", "hai")
	s.Say("an", "ba")

	all := s.Chat()
	if all[0].Seq != 1 || all[1].Seq != 2 || all[2].Seq != 3 {
		t.Fatalf("seqs = %d,%d,%d, want 1,2,3", all[0].Seq, all[1].Seq, all[2].Seq)
	}

	got := s.ChatAfter(1)
	if len(got) != 2 || got[0].Text != "hai" {
		t.Fatalf("ChatAfter(1) = %+v, want the two messages after seq 1", got)
	}
	if got := s.ChatAfter(3); len(got) != 0 {
		t.Errorf("ChatAfter(3) returned %d messages after the last seq", len(got))
	}
}

// The transcript is capped at 500 messages but seq keeps counting: a reader's
// cursor must stay valid even after old messages have been dropped.
func TestChatSeqSurvivesTheCap(t *testing.T) {
	s := NewSession("s1", "file.go:12")
	for i := 0; i < 520; i++ {
		s.Say("an", "dòng")
	}

	chat := s.Chat()
	if last := chat[len(chat)-1].Seq; last != 520 {
		t.Errorf("last seq = %d after 520 says, want 520", last)
	}
	if got := s.ChatAfter(519); len(got) != 1 {
		t.Errorf("ChatAfter(519) = %d messages, want exactly the newest one", len(got))
	}
}

// The long poll that turns polling into conversation: a waiting call must
// return as soon as someone speaks, not at the end of its window.
func TestWaitForChatReturnsAsSoonAsSomeoneSpeaks(t *testing.T) {
	h, url, id := openMCPSession(t)
	key := joinMCP(t, url, "mai@laptop/claude-code")
	session, _ := h.Session(id)

	go func() {
		time.Sleep(150 * time.Millisecond)
		session.Say("trọng tài", "mai ơi, dòng 17 sai chỗ nào?")
	}()

	start := time.Now()
	text, isErr := callTool(t, url, "wait_for_chat", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key,
		"after_seq": 0, "wait_seconds": 10,
	})
	if isErr {
		t.Fatalf("wait_for_chat errored: %s", text)
	}
	if !strings.Contains(text, "dòng 17") {
		t.Fatalf("the message that woke the poll is missing from the reply:\n%s", text)
	}
	if !strings.Contains(text, `"last_seq": 1`) {
		t.Errorf("reply carries no last_seq cursor for the next call:\n%s", text)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("long poll slept %s despite a message at 150ms", elapsed)
	}
}

// after_seq is what stops an agent from re-reading — and re-answering — the
// same messages every turn.
func TestWaitForChatSkipsWhatWasAlreadySeen(t *testing.T) {
	h, url, id := openMCPSession(t)
	key := joinMCP(t, url, "mai@laptop/claude-code")
	session, _ := h.Session(id)
	session.Say("trọng tài", "tin cũ đã đọc")
	session.Say("trọng tài", "tin mới chưa đọc")

	text, isErr := callTool(t, url, "wait_for_chat", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key,
		"after_seq": 1, "wait_seconds": 5,
	})
	if isErr {
		t.Fatalf("wait_for_chat errored: %s", text)
	}
	if strings.Contains(text, "tin cũ đã đọc") {
		t.Errorf("a message at or below after_seq came back:\n%s", text)
	}
	if !strings.Contains(text, "tin mới chưa đọc") {
		t.Errorf("the newer message is missing:\n%s", text)
	}
}

// Silence is an answer, not an error: the agent is told to just call again.
func TestWaitForChatTimesOutQuietly(t *testing.T) {
	_, url, _ := openMCPSession(t)
	key := joinMCP(t, url, "mai@laptop/claude-code")

	text, isErr := callTool(t, url, "wait_for_chat", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key,
		"after_seq": 0, "wait_seconds": 1,
	})
	if isErr {
		t.Fatalf("a quiet window came back as an error: %s", text)
	}
	if !strings.Contains(text, "No new chat") {
		t.Errorf("timeout reply does not say to keep listening:\n%s", text)
	}
}

// Chat is scoped to the roster like everything else on the MCP door.
func TestWaitForChatRequiresJoin(t *testing.T) {
	_, url, _ := openMCPSession(t)
	if text, isErr := callTool(t, url, "wait_for_chat", map[string]any{
		"author": "nobody@nowhere/agent", "after_seq": 0,
	}); !isErr {
		t.Fatalf("an unjoined caller read the chat:\n%s", text)
	}
}

// awaitChatContaining reads state broadcasts until one carries a chat message
// with the wanted text.
func awaitChatContaining(t *testing.T, a *agent, want string) Message {
	t.Helper()
	_ = a.ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var m Message
		if err := a.ws.ReadJSON(&m); err != nil {
			t.Fatalf("waiting for chat %q: %v", want, err)
		}
		for _, c := range m.Chat {
			if strings.Contains(c.Text, want) {
				return m
			}
		}
	}
}

// A watcher hears everything said in the session, but adjudication never
// waits for it: the session must reach reveal on the participant's finish
// alone, with the watcher still attached and silent.
func TestWatcherHearsChatButNeverHoldsTheSessionOpen(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	h.SoloHold = 0
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	an, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	an.send(Message{Type: MsgJoin, Author: "an", Provider: "claude"})
	an.await(MsgState)

	w, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	w.send(Message{Type: MsgWatch, Author: "khán giả"})
	w.await(MsgState)

	an.send(Message{Type: MsgChat, Text: "tôi bắt đầu xem dòng 17 đây"})
	awaitChatContaining(t, w, "dòng 17")

	an.send(Message{Type: MsgClaim, Claim: &Claim{
		Subject: "x", Assertion: "the defect is real", Falsifier: "reproduces",
	}})
	an.await(MsgState)
	an.send(Message{Type: MsgDone})

	an.awaitPhase(PhaseReveal)
	// The watcher rides along to reveal and only now sees the claims.
	if state := w.awaitPhase(PhaseReveal); len(state.Claims) != 1 {
		t.Fatalf("after reveal the watcher sees %d claims, want 1", len(state.Claims))
	}
}

// A watcher's author is whatever it typed, so it earns talk but nothing that
// lands in the durable record under that name.
func TestWatcherMayChatButNotClaimFinishOrRemark(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	w, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	w.send(Message{Type: MsgWatch, Author: "an"})
	w.await(MsgState)

	w.send(Message{Type: MsgClaim, Claim: &Claim{Subject: "x", Assertion: "forged"}})
	if got := w.await(MsgError); !strings.Contains(got.Error, "watcher") {
		t.Fatalf("claim from a watcher gave %q, want a watcher refusal", got.Error)
	}
	w.send(Message{Type: MsgDone})
	if got := w.await(MsgError); !strings.Contains(got.Error, "watcher") {
		t.Fatalf("done from a watcher gave %q, want a watcher refusal", got.Error)
	}
	w.send(Message{Type: MsgRemark, Claim: &Claim{ID: "c1"}, Text: "x"})
	if got := w.await(MsgError); !strings.Contains(got.Error, "watcher") {
		t.Fatalf("remark from a watcher gave %q, want a watcher refusal", got.Error)
	}

	// Chat still works, under the declared name.
	w.send(Message{Type: MsgChat, Text: "nhưng nói chuyện thì được"})
	awaitChatContaining(t, w, "nói chuyện thì được")
}

// Naming a real participant must not open their blind claims: the websocket
// door has no participant key, so a watcher's name proves nothing.
func TestWatcherNamingAParticipantSeesNoBlindClaims(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	an, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	an.send(Message{Type: MsgJoin, Author: "an", Provider: "claude"})
	an.await(MsgState)
	an.send(Message{Type: MsgClaim, Claim: &Claim{
		Subject: "x", Assertion: "AN-BLIND-CLAIM", Falsifier: "reproduces",
	}})
	an.await(MsgState)

	spy, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	spy.send(Message{Type: MsgWatch, Author: "an"})
	if ack := spy.await(MsgState); len(ack.Claims) != 0 {
		t.Fatalf("a watcher named after a participant was handed %d blind claims", len(ack.Claims))
	}

	// A broadcast during collect must stay empty for the spy too.
	an.send(Message{Type: MsgChat, Text: "vẫn đang thu thập"})
	if state := awaitChatContaining(t, spy, "thu thập"); len(state.Claims) != 0 {
		t.Fatalf("a collect-phase broadcast leaked %d claims to a watcher", len(state.Claims))
	}
}

// --ping used to join, permanently taking the author's seat: the real claim
// run that followed, under the same default author, was then refused with
// "has already joined". Watching takes no seat, so probe-then-join works.
func TestWatchThenJoinWithTheSameAuthorWorks(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	h.SoloHold = 0
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	probe := &Client{HostURL: wsURL, SessionID: id, Token: token, Author: "an@máy/claude", Provider: "claude"}
	if err := probe.Watch(); err != nil {
		t.Fatalf("ping-style watch: %v", err)
	}
	probe.Close()

	run := &Client{HostURL: wsURL, SessionID: id, Token: token, Author: "an@máy/claude", Provider: "claude"}
	if err := run.Connect(); err != nil {
		t.Fatalf("join after a watch by the same author: %v", err)
	}
	run.Close()
}

// Listen is the CLI agent's ear: one JSON line per message, history included,
// and no duplicates even though every broadcast carries the full transcript.
func TestListenStreamsChatAsJSONLinesWithoutDuplicates(t *testing.T) {
	h, srv := hostFixture(t)
	h.SoloHold = 0
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// Said before the listener attaches: must still be printed.
	if err := h.Say(id, "trọng tài", "tin nhắn cũ"); err != nil {
		t.Fatal(err)
	}

	listener := &Client{HostURL: wsURL, SessionID: id, Token: token, Author: "nghe", Provider: "claude"}
	if err := listener.Watch(); err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- listener.Listen(&buf, 1500*time.Millisecond) }()

	// Two more messages, two more broadcasts — each carrying all messages so
	// far, which is exactly the duplication Listen must squash by seq.
	if err := h.Say(id, "trọng tài", "các agent thấy gì?"); err != nil {
		t.Fatal(err)
	}
	if err := h.Say(id, "an@máy/claude", "tôi thấy lỗi ở dòng 17"); err != nil {
		t.Fatal(err)
	}

	if err := <-done; err != nil {
		t.Fatalf("listen: %v", err)
	}

	var msgs []ChatMessage
	sc := bufio.NewScanner(strings.NewReader(buf.String()))
	for sc.Scan() {
		var m ChatMessage
		if err := json.Unmarshal([]byte(sc.Text()), &m); err != nil {
			t.Fatalf("line %q is not one JSON object: %v", sc.Text(), err)
		}
		if m.Author != "" { // the session_closed line has no author
			msgs = append(msgs, m)
		}
	}
	if len(msgs) != 3 {
		t.Fatalf("printed %d chat lines, want 3 (history + 2 live, no repeats):\n%s", len(msgs), buf.String())
	}
	if msgs[0].Text != "tin nhắn cũ" || msgs[0].Seq != 1 {
		t.Errorf("history line = %+v, want the pre-attach message first with seq 1", msgs[0])
	}
	if msgs[2].Author != "an@máy/claude" || msgs[2].Seq != 3 {
		t.Errorf("last line = %+v, want the second live message with seq 3", msgs[2])
	}
}
