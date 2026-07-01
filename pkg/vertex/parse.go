// Package vertex defines interfaces for Vertex AI touchpoints (parse, judge, ground).
package vertex

import "context"

// Section is a parsed document section.
type Section struct {
	HeadingPath string
	Text        string
}

// ParseResult holds the output of document parsing.
type ParseResult struct {
	Sections []Section
}

// Parser parses documents into sections using Doc AI Layout Parser.
type Parser interface {
	Parse(ctx context.Context, content []byte, contentType string) (ParseResult, error)
}
