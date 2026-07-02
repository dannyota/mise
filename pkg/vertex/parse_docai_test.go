package vertex

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLayoutSections exercises the pure block-tree → Section mapping against a
// canned Doc AI Layout Parser response, offline.
func TestLayoutSections(t *testing.T) {
	raw, err := os.ReadFile("testdata/docai_layout_response.json")
	if err != nil {
		t.Fatal(err)
	}
	var resp processResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decoding canned response: %v", err)
	}

	got := layoutSections(resp.Document.DocumentLayout.Blocks)

	chI := "Chương I QUY ĐỊNH CHUNG"
	chII := "Chương II ĐIỀU KHOẢN THI HÀNH"
	want := []Section{
		{HeadingPath: "", Text: "THÔNG TƯ Quy định về an toàn hệ thống thông tin"},
		{
			HeadingPath: chI + " > Điều 1. Phạm vi điều chỉnh",
			Text:        "Thông tư này quy định về bảo đảm an toàn hệ thống thông tin trong hoạt động ngân hàng.",
		},
		{HeadingPath: chI + " > Điều 1. Phạm vi điều chỉnh", Text: "a) Ngân hàng thương mại;"},
		{HeadingPath: chI + " > Điều 1. Phạm vi điều chỉnh", Text: "b) Tổ chức tín dụng phi ngân hàng;"},
		{
			HeadingPath: chI + " > Điều 2. Đối tượng áp dụng",
			Text:        "Thông tư này áp dụng đối với các tổ chức tín dụng.",
		},
		// heading-1 "Chương II" must reset the deeper heading-2 path.
		{HeadingPath: chII, Text: "Hệ thống"},
		{HeadingPath: chII, Text: "Cấp độ 3"},
	}

	if len(got) != len(want) {
		t.Fatalf("layoutSections() returned %d sections, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("section[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLayoutSectionsEmptyDocument(t *testing.T) {
	if got := layoutSections(nil); len(got) != 0 {
		t.Errorf("layoutSections(nil) = %+v, want empty", got)
	}
}

// testDocAIParser returns a parser aimed at srv with a negligible backoff.
func testDocAIParser(srv *httptest.Server) *docAIParser {
	return &docAIParser{
		endpoint: srv.URL,
		client:   srv.Client(),
		backoff:  time.Millisecond,
	}
}

// cannedOK is a minimal successful :process response body.
const cannedOK = `{"document":{"documentLayout":{"blocks":[
	{"textBlock":{"text":"Điều 1. Phạm vi","type":"heading-1","blocks":[
		{"textBlock":{"text":"Nội dung điều một.","type":"paragraph"}}]}}]}}}`

func TestDocAIParserMapsResponse(t *testing.T) {
	var gotReq struct {
		RawDocument struct {
			Content  []byte `json:"content"`
			MIMEType string `json:"mimeType"`
		} `json:"rawDocument"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cannedOK))
	}))
	defer srv.Close()

	result, err := testDocAIParser(srv).Parse(context.Background(), []byte("%PDF-1.4"), "application/pdf")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if string(gotReq.RawDocument.Content) != "%PDF-1.4" {
		t.Errorf("request content = %q, want %q (base64 round trip)", gotReq.RawDocument.Content, "%PDF-1.4")
	}
	if gotReq.RawDocument.MIMEType != "application/pdf" {
		t.Errorf("request mimeType = %q, want application/pdf", gotReq.RawDocument.MIMEType)
	}
	want := []Section{{HeadingPath: "Điều 1. Phạm vi", Text: "Nội dung điều một."}}
	if len(result.Sections) != 1 || result.Sections[0] != want[0] {
		t.Errorf("Parse() sections = %+v, want %+v", result.Sections, want)
	}
}

func TestDocAIParserRetriesThrottleAndServerErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(cannedOK))
		}
	}))
	defer srv.Close()

	result, err := testDocAIParser(srv).Parse(context.Background(), []byte("%PDF"), "application/pdf")
	if err != nil {
		t.Fatalf("Parse() error = %v, want success after retries", err)
	}
	if calls != 4 {
		t.Errorf("server calls = %d, want 4 (initial + 3 retries)", calls)
	}
	if len(result.Sections) != 1 {
		t.Errorf("Parse() sections = %+v, want 1 section", result.Sections)
	}
}

func TestDocAIParserGivesUpAfterRetryBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := testDocAIParser(srv).Parse(context.Background(), []byte("%PDF"), "application/pdf")
	if err == nil {
		t.Fatal("Parse() error = nil, want error after exhausted retries")
	}
	if calls != 4 {
		t.Errorf("server calls = %d, want 4 (initial + 3 retries)", calls)
	}
}

func TestDocAIParserDoesNotRetryClientErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testDocAIParser(srv).Parse(context.Background(), []byte("%PDF"), "application/pdf")
	if err == nil {
		t.Fatal("Parse() error = nil, want error on 400")
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (4xx is not retryable)", calls)
	}
}

func TestNewDocAIParserRejectsEmptyArgs(t *testing.T) {
	tests := []struct {
		name                           string
		project, location, processorID string
	}{
		{name: "empty project", location: "us", processorID: "p1"},
		{name: "empty location", project: "proj", processorID: "p1"},
		{name: "empty processor", project: "proj", location: "us"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewDocAIParser(tt.project, tt.location, tt.processorID); err == nil {
				t.Error("NewDocAIParser() error = nil, want error")
			}
		})
	}
}

// TestDocAIParserLive hits the real Doc AI Layout Parser; it is gated on
// DOCAI_PROCESSOR_ID so CI and offline runs skip it.
func TestDocAIParserLive(t *testing.T) {
	processorID := os.Getenv("DOCAI_PROCESSOR_ID")
	if processorID == "" {
		t.Skip("DOCAI_PROCESSOR_ID not set; skipping live Doc AI test")
	}
	project := cmp.Or(os.Getenv("DOCAI_PROJECT"), os.Getenv("GCP_PROJECT"))
	if project == "" {
		t.Skip("DOCAI_PROJECT / GCP_PROJECT not set; skipping live Doc AI test")
	}
	location := cmp.Or(os.Getenv("DOCAI_LOCATION"), "us")

	pdf, err := os.ReadFile("testdata/minimal.pdf")
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewDocAIParser(project, location, processorID)
	if err != nil {
		t.Fatalf("NewDocAIParser() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := p.Parse(ctx, pdf, "application/pdf")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("Parse() returned no sections for the test PDF")
	}
	var all strings.Builder
	for _, s := range result.Sections {
		all.WriteString(s.Text)
		all.WriteByte('\n')
	}
	if !strings.Contains(all.String(), "mise") {
		t.Errorf("extracted text does not contain %q: %q", "mise", all.String())
	}
}
