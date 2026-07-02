package vbpl

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"danny.vn/mise/pkg/ingest"
)

// fileEntry is one descriptor from the files endpoint. presignedUrl is a
// ready-to-GET S3 URL (FPT Cloud, ~24h expiry). relatedType is informative:
// 1=official source/original file, 2=downloadable legal/support file, 4/5=HTML
// bodies. Main-vs-appendix/scan still comes from filename and extension.
type fileEntry struct {
	FileName     string `json:"fileName"`
	PresignedURL string `json:"presignedUrl"`
	Size         int64  `json:"size"`
	RelatedType  int    `json:"relatedType"`
}

type filesResponse struct {
	Success bool        `json:"success"`
	Data    []fileEntry `json:"data"`
}

// preferredFiles selects every downloadable legal file mise can preserve: DOCX,
// legacy DOC, and PDF. It excludes the inline HTML file (already carried in
// DiscoveredDoc.HTML). Legacy .doc is preserved as source evidence and can be
// rendered through the LibreOffice PDF bridge during extraction. When a document
// has DOCX/DOC/HTML and a scanned "Văn bản gốc" PDF, both are kept: Fetch
// preserves source evidence, while Extract later chooses DOCX -> HTML -> DOC ->
// PDF/OCR for extraction quality. Text files are ordered main body first, then
// support material, then PDF, so ordinal 0 remains the preferred text evidence
// when a main DOCX/DOC exists.
func preferredFiles(entries []fileEntry) []ingest.FileRef {
	var textFiles, pdf []ingest.FileRef
	for _, e := range entries {
		if strings.TrimSpace(e.PresignedURL) == "" {
			continue
		}
		ext := fileExt(e.FileName)
		ref := ingest.FileRef{
			URL:      e.PresignedURL,
			Name:     e.FileName,
			Ext:      ext,
			Kind:     fileKind(e.FileName, ext),
			MIMEType: mimeForExt(ext),
		}
		switch ext {
		case "docx", "doc":
			textFiles = append(textFiles, ref)
		case "pdf":
			pdf = append(pdf, ref)
		default:
			// Inline *.html and anything else: skip.
		}
	}
	sortTextFiles(textFiles)
	sortFilesMainFirst(pdf)
	out := append([]ingest.FileRef{}, textFiles...)
	return append(out, pdf...)
}

func sortTextFiles(files []ingest.FileRef) {
	sort.SliceStable(files, func(i, j int) bool {
		if ri, rj := fileKindRank(files[i].Kind), fileKindRank(files[j].Kind); ri != rj {
			return ri < rj
		}
		return fileFormatRank(files[i].Ext) < fileFormatRank(files[j].Ext)
	})
}

func sortFilesMainFirst(files []ingest.FileRef) {
	sort.SliceStable(files, func(i, j int) bool {
		return fileKindRank(files[i].Kind) < fileKindRank(files[j].Kind)
	})
}

func fileFormatRank(ext string) int {
	switch ext {
	case "docx":
		return 0
	case "doc":
		return 1
	case "pdf":
		return 2
	default:
		return 9
	}
}

func fileKindRank(kind string) int {
	switch kind {
	case "main":
		return 0
	case "appendix":
		return 1
	case "attachment":
		return 2
	case "version_snapshot":
		return 3
	case "original_scan":
		return 4
	default:
		return 9
	}
}

func fileKind(name, ext string) string {
	if isAppendixName(name) {
		return "appendix"
	}
	if ext == "pdf" {
		return "original_scan"
	}
	return "main"
}

// isAppendixName reports whether a file is an appendix/attachment rather than the
// main body. VBPL names appendices with both accented and unaccented Vietnamese
// ("Phụ lục", "Phu luc"), forms ("Biểu mẫu"), and attached lists ("Danh mục …
// ban hành kèm theo"). The main body normally carries the document type
// ("Thông tư …", "Nghị định …").
func isAppendixName(name string) bool {
	n := foldVietnamese(name)
	return strings.Contains(n, "phu luc") ||
		strings.Contains(n, "phuluc") ||
		strings.Contains(n, "bieu mau") ||
		strings.Contains(n, "bieumau") ||
		strings.Contains(n, "mau so") ||
		strings.Contains(n, "danh muc") ||
		strings.Contains(n, "ban hanh kem theo") ||
		strings.Contains(n, "dinh kem")
}

func foldVietnamese(s string) string {
	decomposed := norm.NFD.String(strings.ToLower(s))
	folded := strings.Map(func(r rune) rune {
		switch {
		case r == 'đ':
			return 'd'
		case unicode.Is(unicode.Mn, r):
			return -1
		default:
			return r
		}
	}, decomposed)
	return norm.NFC.String(folded)
}

// fileExt returns the lowercase extension of a file name without the dot
// ("Thông tư 09-2020-TT-NHNN.docx" -> "docx"); empty when there is none.
func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

// mimeForExt is a best-effort content type for a file extension; empty when
// unknown (the downloaded response's own Content-Type is authoritative).
func mimeForExt(ext string) string {
	switch ext {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "doc":
		return "application/msword"
	default:
		return ""
	}
}
