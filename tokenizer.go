package machine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/maddiesch/failable"
)

// SourceError is returned when the source code reader encounters and error
type SourceError struct {
	Message string
	Line    uint32
	Column  uint32
}

func (e *SourceError) Error() string {
	return fmt.Sprintf("Source error (Ln %d, Col %d): %s", e.Line, e.Column, e.Message)
}

func tokenize(ctx context.Context, comp *compiler, fail failable.FailFunc) {
	scanner := comp.scanner()

	var line uint32

	for scanner.Scan() {
		line++

		if len(comp.Tokens) > 0 {
			last := comp.Tokens[len(comp.Tokens)-1]

			if last.Kind != TokenIL_END {
				fail(&SourceError{
					Line:    last.Line,
					Column:  last.Column,
					Message: fmt.Sprintf("Line must end with a `;`"),
				})
			}
		}

		runes := bufio.NewScanner(bytes.NewReader(scanner.Bytes()))
		runes.Split(bufio.ScanRunes)

		type value struct {
			buf    strings.Builder
			startL uint32
			startC uint32
			store  bool
		}

		appendToken := func(k TokenIL_Kind, col uint32) {
			comp.Tokens = append(comp.Tokens, &TokenIL{
				Kind:   k,
				Line:   line,
				Column: col,
			})
		}

		appendValue := func(v *value) {
			comp.Tokens = append(comp.Tokens, &TokenIL{
				Kind:   TokenIL_VALUE,
				Value:  strings.TrimSpace(v.buf.String()),
				Line:   v.startL,
				Column: v.startC,
			})
		}

		var col uint32
		var val *value

		for runes.Scan() {
			col++

			var breaking bool
			var completing bool
			kind := TokenIL_NONE

			r, _ := utf8.DecodeLastRune(runes.Bytes())
			if r == utf8.RuneError {
				fail(&SourceError{
					Line:    line,
					Column:  col,
					Message: "failed to decode UTF-8 character",
				})
			}

			switch r {
			case ';':
				breaking = true
				if col != 1 { // The first character of the line is a comment. Skip...
					kind = TokenIL_END
				}
			case '(':
				completing = true
				kind = TokenIL_OPEN
			case ')':
				completing = true
				kind = TokenIL_CLOSE
			case ' ':
				completing = true
			case '|':
				completing = true
				kind = TokenIL_PIPE
			case '.':
				completing = true
				kind = TokenIL_DOT
			case '=':
				completing = true
				kind = TokenIL_ASSIGN
			case '$':
				completing = true
				kind = TokenIL_VAR
			default:
				if val == nil {
					val = &value{
						buf:    strings.Builder{},
						startL: line,
						startC: col,
						store:  false,
					}
				}
				val.buf.WriteRune(r)
			}

			if val != nil && (completing || breaking) {
				appendValue(val)
				val = nil
			}

			if kind != TokenIL_NONE {
				appendToken(kind, col)
			}

			if breaking {
				break
			}
		}

		if val != nil {
			appendValue(val)
		}
	}
}
