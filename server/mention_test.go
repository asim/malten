package server

import "testing"

func TestMentionPrompt(t *testing.T) {
	for _, tc := range []struct{ text, prompt string }{
		{"What is the true religion @malten", "What is the true religion"},
		{"Please @MaLtEn explain this.", "Please explain this."},
		{"What does this mean, @malten?", "What does this mean?"},
		{"@Malten, explain this", "explain this"},
		{"/malten: explain this", "explain this"},
		{"First line\nsecond line @malten?", "First line\nsecond line?"},
		{"كيف أتدبر القرآن @malten؟", "كيف أتدبر القرآن؟"},
		{"  @malten  ", ""},
	} {
		if got, ok := MentionPrompt(tc.text); !ok || got != tc.prompt {
			t.Errorf("%q: got (%q, %t), want %q", tc.text, got, ok, tc.prompt)
		}
	}
	for _, text := range []string{"a reflection", "name@malten.me", "https://example.com/@malten", "@malten.me", "@maltens", "@malten_alt", "use /malten here"} {
		if _, ok := MentionPrompt(text); ok {
			t.Errorf("matched %q", text)
		}
	}
}

func TestMentionAdmission(t *testing.T) {
	s := New()
	post := func(text string) error { return s.postMessage(NewMessage(text, DefaultStream), "visitor") }
	if err := post("Hello @malten"); err != errAgentUnavailable {
		t.Fatalf("disabled agent: %v", err)
	}
	s.Commands = make(chan *Message, 1)
	if err := post("@malten"); err != errNoQuestion {
		t.Fatalf("bare mention: %v", err)
	}
	if err := post("@malten explain this, @malten"); err != nil {
		t.Fatal(err)
	}
	if err := post("Another question @malten"); err != errAgentUnavailable {
		t.Fatalf("busy agent: %v", err)
	}
	if err := post("Just a reflection"); err != nil {
		t.Fatal(err)
	}
	if len(s.Commands) != 1 || len(s.Retrieve("", DefaultStream, 1, 0, 10)) != 2 {
		t.Fatal("wrong number of questions or published messages")
	}
}
