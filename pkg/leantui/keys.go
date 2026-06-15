package leantui

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

type keyType int

const (
	keyNone keyType = iota
	keyRune
	keyPaste
	keyEnter
	keyAltEnter // insert a literal newline (multi-line input)
	keyTab
	keyShiftTab
	keyBackspace
	keyDelete
	keyUp
	keyDown
	keyLeft
	keyRight
	keyWordLeft
	keyWordRight
	keyHome
	keyEnd
	keyEsc
	keyCtrlC
	keyCtrlD
	keyCtrlU // delete to start of line
	keyCtrlK // delete to end of line
	keyCtrlW // delete word backwards
	keyCtrlL // redraw
)

// key is a single decoded keyboard event. For keyRune and keyPaste the decoded
// characters are carried in runes; every other key type carries no payload.
type key struct {
	typ   keyType
	runes []rune
}

var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// inputParser turns raw terminal bytes into key events. It is stateful only to
// reassemble bracketed-paste payloads, which may span several reads.
type inputParser struct {
	inPaste bool
	paste   []rune
}

func (p *inputParser) feed(b []byte) []key {
	var out []key
	for len(b) > 0 {
		if p.inPaste {
			idx := bytes.Index(b, pasteEnd)
			if idx < 0 {
				p.paste = append(p.paste, []rune(string(b))...)
				return out
			}
			p.paste = append(p.paste, []rune(string(b[:idx]))...)
			out = append(out, key{typ: keyPaste, runes: p.paste})
			p.paste = nil
			p.inPaste = false
			b = b[idx+len(pasteEnd):]
			continue
		}

		idx := bytes.Index(b, pasteStart)
		if idx < 0 {
			out = append(out, parseChunk(b)...)
			return out
		}
		out = append(out, parseChunk(b[:idx])...)
		p.inPaste = true
		b = b[idx+len(pasteStart):]
	}
	return out
}

// parseChunk decodes a run of bytes that contains no bracketed-paste markers.
// Escape sequences are assumed to arrive atomically within a single read, so a
// trailing lone ESC is reported as the Escape key.
func parseChunk(b []byte) []key {
	var out []key
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			if i == len(b)-1 {
				out = append(out, key{typ: keyEsc})
				i++
				continue
			}
			n, k := parseEscape(b[i:])
			if k.typ != keyNone {
				out = append(out, k)
			}
			i += n
		case c == '\r' || c == '\n':
			out = append(out, key{typ: keyEnter})
			i++
		case c == '\t':
			out = append(out, key{typ: keyTab})
			i++
		case c == 0x7f, c == 0x08:
			out = append(out, key{typ: keyBackspace})
			i++
		case c == 0x03:
			out = append(out, key{typ: keyCtrlC})
			i++
		case c == 0x04:
			out = append(out, key{typ: keyCtrlD})
			i++
		case c == 0x01:
			out = append(out, key{typ: keyHome})
			i++
		case c == 0x05:
			out = append(out, key{typ: keyEnd})
			i++
		case c == 0x15:
			out = append(out, key{typ: keyCtrlU})
			i++
		case c == 0x0b:
			out = append(out, key{typ: keyCtrlK})
			i++
		case c == 0x17:
			out = append(out, key{typ: keyCtrlW})
			i++
		case c == 0x0c:
			out = append(out, key{typ: keyCtrlL})
			i++
		case c < 0x20:
			i++ // other control bytes are ignored
		default:
			r, size := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && size <= 1 {
				i++
				continue
			}
			out = append(out, key{typ: keyRune, runes: []rune{r}})
			i += size
		}
	}
	return out
}

func parseEscape(b []byte) (int, key) {
	if len(b) < 2 {
		return 1, key{typ: keyEsc}
	}
	switch b[1] {
	case '[':
		return parseCSI(b)
	case 'O':
		if len(b) >= 3 {
			switch b[2] {
			case 'A':
				return 3, key{typ: keyUp}
			case 'B':
				return 3, key{typ: keyDown}
			case 'C':
				return 3, key{typ: keyRight}
			case 'D':
				return 3, key{typ: keyLeft}
			case 'H':
				return 3, key{typ: keyHome}
			case 'F':
				return 3, key{typ: keyEnd}
			}
		}
		return 2, key{typ: keyEsc}
	case 'b':
		return 2, key{typ: keyWordLeft}
	case 'f':
		return 2, key{typ: keyWordRight}
	case 0x7f, 0x08:
		return 2, key{typ: keyCtrlW} // Alt+Backspace deletes a word
	case '\r', '\n':
		return 2, key{typ: keyAltEnter}
	default:
		// Unhandled Alt+<key> combinations are swallowed so they do not insert
		// stray characters into the input.
		_, size := utf8.DecodeRune(b[1:])
		if size < 1 {
			size = 1
		}
		return 1 + size, key{typ: keyNone}
	}
}

func parseCSI(b []byte) (int, key) {
	j := 2
	for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
		j++
	}
	if j >= len(b) {
		return len(b), key{typ: keyNone} // incomplete sequence
	}
	final := b[j]
	params := string(b[2:j])
	consumed := j + 1

	modifier := func() string {
		parts := strings.Split(params, ";")
		if len(parts) >= 2 {
			return parts[1]
		}
		return ""
	}
	wordMod := func() bool {
		switch modifier() {
		case "5", "3", "2": // ctrl / alt / shift
			return true
		default:
			return false
		}
	}

	switch final {
	case 'A':
		return consumed, key{typ: keyUp}
	case 'B':
		return consumed, key{typ: keyDown}
	case 'C':
		if wordMod() {
			return consumed, key{typ: keyWordRight}
		}
		return consumed, key{typ: keyRight}
	case 'D':
		if wordMod() {
			return consumed, key{typ: keyWordLeft}
		}
		return consumed, key{typ: keyLeft}
	case 'H':
		return consumed, key{typ: keyHome}
	case 'F':
		return consumed, key{typ: keyEnd}
	case 'Z':
		return consumed, key{typ: keyShiftTab}
	case '~':
		switch n, _ := strconv.Atoi(strings.SplitN(params, ";", 2)[0]); n {
		case 1, 7:
			return consumed, key{typ: keyHome}
		case 4, 8:
			return consumed, key{typ: keyEnd}
		case 3:
			return consumed, key{typ: keyDelete}
		}
	}
	return consumed, key{typ: keyNone}
}
