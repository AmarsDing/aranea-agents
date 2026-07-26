package agent

// streamSubTaskParser incrementally parses a streaming JSON array of subtask
// objects, extracting complete top-level objects as they close.
//
// The LLM decomposition output is a JSON array: [{"id":"st_1",...},{"id":"st_2",...}].
// As text arrives via Feed(), the parser tracks brace depth (string-aware) and
// returns each complete {...} object as a raw JSON string.
type streamSubTaskParser struct {
	depth   int  // brace depth inside the array (0 = not inside an object)
	inArr   bool // seen the opening '['
	inStr   bool // inside a JSON string
	escape  bool // previous byte was '\' inside a string
	start   int  // byte offset in buf where current object started
	buf     []byte
}

func newStreamSubTaskParser() *streamSubTaskParser {
	return &streamSubTaskParser{}
}

// Feed processes a chunk of streaming text and returns any complete subtask
// JSON object strings that closed during this chunk.
func (p *streamSubTaskParser) Feed(chunk string) []string {
	var complete []string
	for i := 0; i < len(chunk); i++ {
		ch := chunk[i]

		if p.escape {
			p.escape = false
			if p.depth > 0 {
				p.buf = append(p.buf, ch)
			}
			continue
		}
		if p.inStr {
			if ch == '\\' {
				p.escape = true
			} else if ch == '"' {
				p.inStr = false
			}
			if p.depth > 0 {
				p.buf = append(p.buf, ch)
			}
			continue
		}
		// Not inside a string.
		switch ch {
		case '"':
			p.inStr = true
			if p.depth > 0 {
				p.buf = append(p.buf, ch)
			}
		case '[':
			if !p.inArr {
				p.inArr = true
			} else if p.depth > 0 {
				p.buf = append(p.buf, ch)
			}
		case '{':
			p.depth++
			if p.depth == 1 {
				// Start of a new top-level object.
				p.buf = p.buf[:0]
				p.start = 0
			}
			p.buf = append(p.buf, ch)
		case '}':
			p.depth--
			p.buf = append(p.buf, ch)
			if p.depth == 0 && p.inArr {
				complete = append(complete, string(p.buf))
				p.buf = p.buf[:0]
			}
		default:
			if p.depth > 0 {
				p.buf = append(p.buf, ch)
			}
		}
	}
	return complete
}
