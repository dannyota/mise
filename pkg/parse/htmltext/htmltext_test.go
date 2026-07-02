package htmltext_test

import (
	"strings"
	"testing"

	"danny.vn/mise/pkg/parse/htmltext"
)

// textTests is the Text case table, package-level to keep TestText short.
var textTests = []struct {
	name string
	in   string
	want string
}{
	{
		name: "paragraphs become separate lines",
		in:   "<p>a</p><p>b</p>",
		want: "a\nb",
	},
	{
		name: "script and style are dropped",
		in:   "<p>x</p><script>var a = 1;</script><style>.c { color: red }</style><p>y</p>",
		want: "x\ny",
	},
	{
		name: "head content is dropped",
		in:   "<html><head><title>Trang chủ</title><meta charset=\"utf-8\"></head><body><p>a</p></body></html>",
		want: "a",
	},
	{
		name: "br breaks the line",
		in:   "<p>a<br>b</p>",
		want: "a\nb",
	},
	{
		name: "entities are decoded",
		in:   "<p>a &amp; b &lt;c&gt;</p>",
		want: "a & b <c>",
	},
	{
		name: "numeric entities decode to vietnamese text",
		in:   "<p>&#272;i&#7873;u 1. Ph&#7841;m vi &#273;i&#7873;u ch&#7881;nh</p>",
		want: "Điều 1. Phạm vi điều chỉnh",
	},
	{
		name: "non-breaking spaces normalize to plain spaces",
		in:   "<p>a&nbsp;&nbsp;b</p>",
		want: "a b",
	},
	{
		name: "blank line runs collapse",
		in:   "<div><p>a</p></div><div></div><p>   </p><div><p>b</p></div>",
		want: "a\nb",
	},
	{
		name: "headings and list items are one line each",
		in:   "<h1>Chương I</h1><h2>Mục 1</h2><ul><li>một</li><li>hai</li></ul>",
		want: "Chương I\nMục 1\nmột\nhai",
	},
	{
		name: "nested divs break lines",
		in:   "<div>a<div>b</div>c</div>",
		want: "a\nb\nc",
	},
	{
		name: "table rows are lines and cells are space separated",
		in:   "<table><tr><td>c1</td><td>c2</td></tr><tr><td>d1</td><td>d2</td></tr></table>",
		want: "c1 c2\nd1 d2",
	},
	{
		name: "inline elements join without breaks",
		in:   "<p><b>Điều</b> <span>1</span>. Phạm <em>vi</em></p>",
		want: "Điều 1. Phạm vi",
	},
	{
		name: "intra-line whitespace collapses",
		in:   "<p>a\n\t   b</p>",
		want: "a b",
	},
	{
		name: "comments are dropped",
		in:   "<p>a</p><!-- hidden --><p>b</p>",
		want: "a\nb",
	},
	{
		name: "unclosed tags are tolerated",
		in:   "<p>a<p>b",
		want: "a\nb",
	},
	{
		name: "empty input",
		in:   "",
		want: "",
	},
	{
		name: "whitespace only",
		in:   "  \n\t ",
		want: "",
	},
}

func TestText(t *testing.T) {
	for _, tt := range textTests {
		t.Run(tt.name, func(t *testing.T) {
			if got := htmltext.Text([]byte(tt.in)); got != tt.want {
				t.Errorf("Text(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTextNoBlankLines(t *testing.T) {
	// The downstream legal-structure parsers are line-by-line state machines:
	// every emitted line must carry content.
	in := "<div><h1>Điều 1</h1><div></div><div><br><br></div><p>Nội dung.</p></div>"
	got := htmltext.Text([]byte(in))
	for i, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == "" {
			t.Errorf("line %d is blank in %q", i, got)
		}
	}
}
