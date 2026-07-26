package claims

import (
	"strings"
	"testing"
)

func TestSayRecordsTalkInOrder(t *testing.T) {
	s := NewSession("s1", "file.go:12")

	s.Say("an", "tôi nghĩ lỗi nằm ở tầng repo")
	s.Say("binh", "để tôi chạy thử test")

	got := s.Chat()
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if got[0].Author != "an" || !strings.Contains(got[1].Text, "chạy thử") {
		t.Errorf("messages out of order or mangled: %+v", got)
	}
	if got[0].At.IsZero() {
		t.Error("no timestamp recorded")
	}
}

func TestSayIgnoresEmptyMessages(t *testing.T) {
	s := NewSession("s1", "file.go:12")

	s.Say("an", "")
	s.Say("an", "   \n\t ")

	if got := s.Chat(); len(got) != 0 {
		t.Errorf("blank messages were stored: %+v", got)
	}
}

// A session left open with a talkative agent must not grow without limit.
func TestChatIsBounded(t *testing.T) {
	s := NewSession("s1", "file.go:12")
	for i := 0; i < 600; i++ {
		s.Say("an", "dòng")
	}

	if got := len(s.Chat()); got > 500 {
		t.Errorf("chat holds %d messages, want it capped at 500", got)
	}
}

// Chat is discussion, not evidence: it must not be able to settle anything.
// Only a falsifier does that.
func TestChatDoesNotAffectClaimsOrPhase(t *testing.T) {
	s := NewSession("s1", "file.go:12")
	before := s.Phase()

	s.Say("an", "tôi chắc chắn đây là lỗi")

	if s.Phase() != before {
		t.Errorf("phase moved from %s to %s because of chat", before, s.Phase())
	}
	if len(s.VisibleTo("an")) != 0 {
		t.Error("chat created a claim")
	}
}

// The copy must be a copy: a caller appending to the returned slice must not
// reach into the session's own state.
func TestChatReturnsACopy(t *testing.T) {
	s := NewSession("s1", "file.go:12")
	s.Say("an", "một")

	got := s.Chat()
	got[0].Text = "đã sửa"

	if s.Chat()[0].Text != "một" {
		t.Error("Chat() handed out the session's own backing array")
	}
}
