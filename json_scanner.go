package jsonextract

import (
	"fmt"
	"strings"
)

type Scanner struct {
	data *[]byte
	pos  int
}

func NewScanner(data *[]byte) *Scanner {
	return &Scanner{data: data, pos: 0}
}

func (s *Scanner) skipWhitespace() {
	for s.pos < len(*s.data) &&
		((*s.data)[s.pos] == ' ' ||
			(*s.data)[s.pos] == '\n' ||
			(*s.data)[s.pos] == '\t') {
		s.pos++
	}
}

func (s *Scanner) More() bool {
	s.skipWhitespace()
	return s.pos < len(*s.data) && (*s.data)[s.pos] != '}' && (*s.data)[s.pos] != ']'
}

func (s *Scanner) SkipValue() {
	t, _ := s.Token()

	if t == StartObject || t == StartArray {
		n := 0
		insideString := false

		for {
			if s.pos >= len(*s.data) {
				return
			}
			c := (*s.data)[s.pos]
			s.pos++

			if insideString {
				if c == '\\' {
					s.pos++ // skip escape character
				}
			} else {
				switch c {
				case '{', '[':
					n++
				case '}', ']':
					n--
					if n <= 0 {
						return
					}
				}
			}
			if c == '"' {
				insideString = !insideString
			}
		}
	}
}

func (s *Scanner) SkipString() {
	s.skipWhitespace()
	if s.pos < len(*s.data) && (*s.data)[s.pos] == '"' {
		s.pos++ // skip opening quote
		for s.pos < len(*s.data) && (*s.data)[s.pos] != '"' {
			if (*s.data)[s.pos] == '\\' {
				s.pos++ // skip escape character
			}
			s.pos++
		}
		if s.pos < len(*s.data) && (*s.data)[s.pos] == '"' {
			s.pos++ // skip closing quote
		}
	}
}

type TokenType int

const (
	NoToken TokenType = iota
	StartObject
	EndObject
	StartArray
	EndArray
	String
	Number
	Boolean
	Null
)

func (t TokenType) String() string {
	switch t {
	case StartObject:
		return "StartObject"
	case EndObject:
		return "EndObject"
	case StartArray:
		return "StartArray"
	case EndArray:
		return "EndArray"
	case String:
		return "String"
	case Number:
		return "Number"
	case Boolean:
		return "Boolean"
	case Null:
		return "Null"
	default:
		return "NoToken"
	}
}

func (s *Scanner) ExpectString() ([]byte, error) {
	t, val := s.Token()
	if t != String {
		return nil, fmt.Errorf("expected String token, got: %s", t)
	}
	return val, nil
}

func (s *Scanner) ExpectEndObject() error {
	t, _ := s.Token()
	if t != EndObject {
		return fmt.Errorf("expected EndObject token, got: %s", t)
	}
	return nil
}

func (s *Scanner) ExpectEndArray() error {
	t, _ := s.Token()
	if t != EndArray {
		return fmt.Errorf("expected EndArray token, got: %s", t)
	}
	return nil
}

func (s *Scanner) Token() (TokenType, []byte) {
	s.skipWhitespace()
	if s.pos >= len(*s.data) {
		return NoToken, nil
	}

	start := s.pos
	c := (*s.data)[s.pos]
	if c == '"' {
		s.SkipString()
		return String, (*s.data)[start+1 : s.pos-1]
	} else if c == ',' || c == ':' {
		s.pos++ // skip comma or colon
		return s.Token()
	} else if c == '{' {
		s.pos++
		return StartObject, nil
	} else if c == '}' {
		s.pos++ // skip closing brace
		return EndObject, nil
	} else if c == '[' {
		s.pos++ // skip opening bracket
		return StartArray, nil
	} else if c == ']' {
		s.pos++ // skip closing bracket
		return EndArray, nil
	} else if c == 'n' {
		s.pos += 4 // skip "null"
		return Null, nil
	} else if c == 't' {
		s.pos += 4 // skip "true"
		return Boolean, (*s.data)[start:s.pos]
	} else if c == 'f' {
		s.pos += 5 // skip "false"
		return Boolean, (*s.data)[start:s.pos]
	} else if (c >= '0' && c <= '9') || c == '-' { // simple number check
		for s.pos < len(*s.data) && ((*s.data)[s.pos] >= '0' && (*s.data)[s.pos] <= '9' || (*s.data)[s.pos] == '.') {
			s.pos++
		}
		return Number, (*s.data)[start:s.pos]
	} else {
		for s.pos < len(*s.data) && !strings.ContainsRune(" \n\t,}]", rune((*s.data)[s.pos])) {
			s.pos++
		}
	}

	return NoToken, nil
}
