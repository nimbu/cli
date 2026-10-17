package output

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

const hostileCell = "evil\x1b]0;pwned\x07\x1b[31mred\x1b]52;c;aGk=\x07\x9b2J\u009b\x7fz"

func assertNoTerminalControls(t *testing.T, out string) {
	t.Helper()
	for _, b := range []string{"\x1b", "\x07", "\x9b", "\u009b", "\x7f"} {
		if strings.Contains(out, b) {
			t.Fatalf("output contains control %q: %q", b, out)
		}
	}
}

func TestTableNeutralizesTerminalControlSequences(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "NAME", "EMAIL")
	tbl.SetMaxWidth(80)
	tbl.Row("1", hostileCell, "a@b")
	tbl.Row("2", "plain", "c@d")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	assertNoTerminalControls(t, out)
	if !strings.Contains(out, "evil�]0;pwned�") {
		t.Fatalf("controls should become U+FFFD, got %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if idx1, idx2 := strings.Index(lines[1], "a@b"), strings.Index(lines[2], "c@d"); idx1 < 0 || len([]rune(lines[1][:idx1])) != len([]rune(lines[2][:idx2])) {
		t.Fatalf("columns misaligned:\n%s", out)
	}
}

func TestWriteDetailNeutralizesTerminalControlSequences(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDetail(&buf, []Field{FAlways("Name", hostileCell+"\x1b[2J"), FAlways("Notes", "a\nb")}); err != nil {
		t.Fatal(err)
	}
	assertNoTerminalControls(t, buf.String())
	if !strings.Contains(buf.String(), "Notes: a\n       b\n") {
		t.Fatalf("multi-line values must keep their newline, got %q", buf.String())
	}
}

func TestSanitizeTerminalKeepsCleanText(t *testing.T) {
	for _, in := range []string{"", "plain", "東京 🎉", "tab\there", "line\nbreak"} {
		if got := sanitizeTerminal(in); got != in {
			t.Fatalf("sanitizeTerminal(%q) = %q", in, got)
		}
	}
}

func TestTableKeepsIdentifierColumnsWhole(t *testing.T) {
	for _, tc := range []struct {
		headers []string
		row     []any
		whole   string
		width   int
	}{
		{[]string{"ID", "FULLPATH", "TITLE", "TEMPLATE", "PUBLISHED"}, []any{"67b4aacc8bf74702611b39f2", "/homepage/channel-overview", "Report of the yearly results", "report.liquid", "no"}, "/homepage/channel-overview", 80},
		{[]string{"KEY", "VALUE"}, []any{"account.activate.explanation_text", "Click the link in the email we sent you to activate the account"}, "account.activate.explanation_text", 60},
		{[]string{"ID", "SLUG", "NAME"}, []any{"5f1e0c3a9b2d4e0012ab34cd", "a-long-product-slug-for-testing", "A long product name that can give way"}, "a-long-product-slug-for-testing", 70},
		{[]string{"ID", "NAME", "EMAIL"}, []any{"5f1e0c3a9b2d4e0012ab34cd", "Firstname Lastname-Doublebarrel", "firstname.lastname@example.com"}, "firstname.lastname@example.com", 70},
	} {
		var buf bytes.Buffer
		tbl := NewTable(&buf, tc.headers...)
		tbl.SetMaxWidth(tc.width)
		tbl.Row(tc.row...)
		if err := tbl.Flush(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), tc.whole) {
			t.Fatalf("%v: identifier %q was cut:\n%s", tc.headers, tc.whole, buf.String())
		}
		if !strings.Contains(buf.String(), "…") {
			t.Fatalf("%v: expected a free-text column to give way:\n%s", tc.headers, buf.String())
		}
		assertMaxLineWidth(t, buf.String(), tc.width)
	}
}

func TestTableCutsFreeTextBeforeURLsAndTimestamps(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "NAME", "PERMALINK", "UPDATED")
	tbl.SetMaxWidth(100)
	tbl.Row("67a480328bf74779512df296", "alpine_form/form_settings.liquid", "https://api.nimbu.io/themes/67a480328bf74779512df296/snippets/alpine_form", "2025-02-18 15:43:55.867")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "2025-02-18 15:43:55.867") || !strings.Contains(out, "alpine_form/form_se") {
		t.Fatalf("timestamp must stay whole and the name readable:\n%s", out)
	}
	assertMaxLineWidth(t, out, 100)
}

func TestWriteTableWideSkipsFitting(t *testing.T) {
	type row struct{ Key, Value string }
	var buf bytes.Buffer
	w := &Writer{Out: &buf, Err: &bytes.Buffer{}, Wide: true}
	ctx := WithWriter(context.Background(), w)
	long := strings.Repeat("x", 300)
	if err := WriteTable(ctx, []row{{"k", long}}, []string{"Key", "Value"}, []string{"KEY", "VALUE"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), long) {
		t.Fatalf("--wide must not truncate, got %q", buf.String())
	}
}
