package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func renderWideTable(t *testing.T, maxWidth int) string {
	t.Helper()
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "SUBDOMAIN", "NAME")
	tbl.SetMaxWidth(maxWidth)
	tbl.Row("5f1e0c3a9b2d4e0012ab34cd", "tokyo", "東京タワー観光ガイドと周辺のおすすめレストラン情報")
	tbl.Row("5f1e0c3a9b2d4e0012ab34ce", "party", "Party planning 🎉🎈 for the whole family and friends")
	tbl.Row("5f1e0c3a9b2d4e0012ab34cf", "zen", "Zenjoy")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func assertGolden(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("table mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestTableAlignsWideCharactersWithoutTruncationWhenPiped(t *testing.T) {
	want := "" +
		"ID                        SUBDOMAIN  NAME\n" +
		"5f1e0c3a9b2d4e0012ab34cd  tokyo      東京タワー観光ガイドと周辺のおすすめレストラン情報\n" +
		"5f1e0c3a9b2d4e0012ab34ce  party      Party planning 🎉🎈 for the whole family and friends\n" +
		"5f1e0c3a9b2d4e0012ab34cf  zen        Zenjoy\n"
	assertGolden(t, renderWideTable(t, 0), want)
}

func TestTableFitsTerminalAt80Columns(t *testing.T) {
	want := "" +
		"ID                        SUBDOMAIN  NAME\n" +
		"5f1e0c3a9b2d4e0012ab34cd  tokyo      東京タワー観光ガイドと周辺のおすすめレスト…\n" +
		"5f1e0c3a9b2d4e0012ab34ce  party      Party planning 🎉🎈 for the whole family a…\n" +
		"5f1e0c3a9b2d4e0012ab34cf  zen        Zenjoy\n"
	got := renderWideTable(t, 80)
	assertGolden(t, got, want)
	assertMaxLineWidth(t, got, 80)
}

func TestTableFitsTerminalAt60Columns(t *testing.T) {
	want := "" +
		"ID                        SUBDOMAIN  NAME\n" +
		"5f1e0c3a9b2d4e0012ab34cd  tokyo      東京タワー観光ガイドと…\n" +
		"5f1e0c3a9b2d4e0012ab34ce  party      Party planning 🎉🎈 fo…\n" +
		"5f1e0c3a9b2d4e0012ab34cf  zen        Zenjoy\n"
	got := renderWideTable(t, 60)
	assertGolden(t, got, want)
	assertMaxLineWidth(t, got, 60)
}

func TestTableNeverTruncatesIDColumns(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "OWNER ID")
	tbl.SetMaxWidth(20)
	tbl.Row("5f1e0c3a9b2d4e0012ab34cd", "5f1e0c3a9b2d4e0012ab34ce")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "…") {
		t.Fatalf("id columns must not be truncated, got %q", buf.String())
	}
}

func TestTableFlattensEmbeddedNewlinesAndTabs(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "KEY", "VALUE")
	tbl.Row("a", "line one\nline\ttwo")
	tbl.Row("b", "x")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}
	want := "KEY  VALUE\na    line one line two\nb    x\n"
	assertGolden(t, buf.String(), want)
}

func TestTableUsesASCIIEllipsis(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "NAME")
	tbl.SetMaxWidth(10)
	tbl.SetEllipsis("...")
	tbl.Row("a very long name")
	if err := tbl.Flush(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, buf.String(), "NAME\na very...\n")
}

func TestWriteDetailAlignsByDisplayWidth(t *testing.T) {
	var buf bytes.Buffer
	err := WriteDetail(&buf, []Field{
		FAlways("名前", "Tokyo"),
		FAlways("ID", "abc"),
		FAlways("Notes", "first\nsecond"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"名前:  Tokyo\n" +
		"ID:    abc\n" +
		"Notes: first\n" +
		"       second\n"
	assertGolden(t, buf.String(), want)
}

func assertMaxLineWidth(t *testing.T, out string, maxWidth int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := ansi.StringWidth(line); w > maxWidth {
			t.Fatalf("line width %d > %d: %q", w, maxWidth, line)
		}
	}
}
