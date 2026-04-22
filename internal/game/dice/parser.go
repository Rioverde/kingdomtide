package dice

// Hand-written tokenizer + recursive-descent parser. No regex. The
// design trades brevity for error-message locality: every rejection
// site produces a 1-indexed column pointing at the offending byte.
//
// Whitespace policy: rejected INSIDE a term. "2d6" is legal, "2 d 6"
// is not. Compound expressions accept whitespace AROUND the join
// operators '+' and '-' but not inside any individual term.
//
// Ambiguity handling:
//
//   - "4d6d1": after NdS, the parser refuses a bare 'd' followed by
//     digits. Legal drop-one-lowest is "4d6dl1" or "4d6dh1".
//   - Keep/drop modifiers ALWAYS require an explicit count. "4d6kh"
//     without a following digit is rejected.
//
// Compound grammar:
//
//	expression       = term_or_constant (('+' | '-') term_or_constant)*
//	term_or_constant = term | digit+
//
// The first element of an expression is a term OR a bare constant:
// "5+2d8" parses ("5" as the leading bareMod, "+2d8" appended). A
// leading unary '+' or '-' is REJECTED; the user should write the
// sign inside the constant (e.g. write "0-1d6", not "-1d6"). The
// column points at the leading sign so the message is actionable.
//
// The parser is cheap — tight loop over bytes, one small allocation
// per termNode. Suitable for startup-time MustParse on hundreds of
// expressions with no measurable cost.

// parser carries cursor state over the expression bytes.
type parser struct {
	src string
	pos int
}

func newParser(src string) *parser {
	return &parser{src: src}
}

// col returns the 1-indexed column of the byte at pos, suitable for
// embedding in ParseError.Column.
func (p *parser) col() int {
	return p.pos + 1
}

// eof reports whether the cursor is past the last byte.
func (p *parser) eof() bool {
	return p.pos >= len(p.src)
}

// peek returns the byte at the cursor or 0 at end-of-input.
func (p *parser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

// peekAt returns the byte offset positions ahead, or 0 at end-of-input.
func (p *parser) peekAt(offset int) byte {
	idx := p.pos + offset
	if idx < 0 || idx >= len(p.src) {
		return 0
	}
	return p.src[idx]
}

// errAt returns a *ParseError anchored at the given 1-indexed column.
func (p *parser) errAt(col int, msg string) error {
	return &ParseError{Expr: p.src, Column: col, Msg: msg}
}

// errHere is sugar for errAt at the current cursor position.
func (p *parser) errHere(msg string) error {
	return p.errAt(p.col(), msg)
}

// isDigit reports whether b is an ASCII digit.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// skipSpaces advances past any run of whitespace bytes. Compound
// expressions permit whitespace around '+' / '-' but inside a term
// the term-local parser still rejects spaces.
func (p *parser) skipSpaces() {
	for !p.eof() && isSpace(p.peek()) {
		p.pos++
	}
}

// readDigits consumes a run of ASCII digits and returns the parsed
// integer plus the column where the digits started. If no digit is
// present the returned startCol is p.col() and the first return is 0,
// ok=false — the caller decides whether that is an error.
func (p *parser) readDigits() (value int, startCol int, ok bool) {
	startCol = p.col()
	if p.eof() || !isDigit(p.peek()) {
		return 0, startCol, false
	}
	v := 0
	for !p.eof() && isDigit(p.peek()) {
		v = v*10 + int(p.peek()-'0')
		if v > maxDieCount && v > maxDieSides {
			// Short-circuit runaway literals; the specific cap check
			// fires after the number is fully consumed but we cap the
			// accumulator here to avoid overflow on pathological input.
			v = maxDieCount + 1
			for !p.eof() && isDigit(p.peek()) {
				p.pos++
			}
			return v, startCol, true
		}
		p.pos++
	}
	return v, startCol, true
}

// parseExpression parses a full dice-notation string. It wraps the
// result in an *exprNode regardless of how many terms appeared — a
// single-term input yields a one-element exprNode so Execute and
// Stats have a uniform shape.
func (p *parser) parseExpression() (node, error) {
	if len(p.src) == 0 {
		return nil, p.errAt(1, "empty expression")
	}

	root := &exprNode{}

	// Reject a leading sign; we don't support unary +/- at the start.
	// "-1d6" and "+1d6" both fail with an actionable message. The user
	// can write "0-1d6" if they genuinely want a negated roll.
	p.skipSpaces()
	if !p.eof() && (p.peek() == '+' || p.peek() == '-') {
		return nil, p.errHere("unexpected leading sign; write 0+term or 0-term instead")
	}

	sign := 1
	for {
		p.skipSpaces()
		if p.eof() {
			break
		}
		// Peek ahead to decide: bare constant or term?
		// A term starts with a digit followed (possibly after more
		// digits) by 'd'/'D', or starts directly with 'd'/'D' (e.g.
		// "d6"). Anything else with leading digits is a bare constant.
		if isBareConstantPrefix(p.src, p.pos) {
			v, col, _ := p.readDigits()
			if v > maxDieCount && v > maxDieSides {
				return nil, p.errAt(col, "constant exceeds safety cap")
			}
			root.bareMod += sign * v
		} else {
			term, err := p.parseTerm()
			if err != nil {
				return nil, err
			}
			root.terms = append(root.terms, term)
			root.signs = append(root.signs, sign)
		}

		p.skipSpaces()
		if p.eof() {
			break
		}
		ch := p.peek()
		if ch == '+' || ch == '-' {
			if ch == '-' {
				sign = -1
			} else {
				sign = 1
			}
			p.pos++
			// A '+' or '-' must be followed (after optional whitespace)
			// by something — term or constant. "1d20+" is rejected.
			p.skipSpaces()
			if p.eof() {
				return nil, p.errAt(p.col(), "expected term or constant after sign")
			}
			next := p.peek()
			if next == '+' || next == '-' {
				return nil, p.errHere("unexpected sign character " + quoteByte(next))
			}
			continue
		}
		return nil, p.errHere("unexpected character " + quoteByte(ch))
	}

	if len(root.terms) == 0 {
		// Bare-constant-only expressions ("5") are useless and almost
		// certainly a user mistake. Reject at the first column.
		return nil, p.errAt(1, "expression contains no dice term")
	}
	return root, nil
}

// isBareConstantPrefix reports whether the bytes at pos begin a bare
// numeric constant. True when digits are followed by a sign, end of
// input, or non-(d/D)-non-space byte. False when digits are directly
// followed by 'd'/'D' (which makes them a term count). Whitespace
// between the digits and a trailing 'd'/'D' is also treated as NOT a
// bare constant so parseTerm can surface the "whitespace not allowed
// inside term" error from a single place.
func isBareConstantPrefix(src string, pos int) bool {
	if pos >= len(src) || !isDigit(src[pos]) {
		return false
	}
	i := pos
	for i < len(src) && isDigit(src[i]) {
		i++
	}
	if i >= len(src) {
		return true
	}
	b := src[i]
	if b == 'd' || b == 'D' {
		return false
	}
	// Peek past whitespace: if it precedes 'd'/'D' we still route
	// through parseTerm so the intra-term whitespace error fires with
	// its proper column.
	if isSpace(b) {
		j := i
		for j < len(src) && isSpace(src[j]) {
			j++
		}
		if j < len(src) && (src[j] == 'd' || src[j] == 'D') {
			return false
		}
	}
	return true
}

// parseTerm parses [count]('d'|'D')sides[modifiers].
func (p *parser) parseTerm() (*termNode, error) {
	if p.eof() {
		return nil, p.errHere("expected dice term")
	}
	// Reject leading whitespace inside the term. Top-level whitespace
	// was already consumed by the caller; anything here is internal.
	if isSpace(p.peek()) {
		return nil, p.errHere("whitespace not allowed inside term")
	}
	count := 1
	countWritten := false
	if isDigit(p.peek()) {
		v, col, _ := p.readDigits()
		if v <= 0 {
			return nil, p.errAt(col, "dice count must be positive")
		}
		if v > maxDieCount {
			return nil, p.errAt(col, "dice count exceeds safety cap")
		}
		count = v
		countWritten = true
	}

	// Whitespace inside a term is rejected even between count and 'd'.
	if !p.eof() && isSpace(p.peek()) {
		return nil, p.errHere("whitespace not allowed inside term")
	}

	// 'd' or 'D' is mandatory.
	if p.eof() || (p.peek() != 'd' && p.peek() != 'D') {
		if !countWritten {
			return nil, p.errHere("expected 'd' or digits")
		}
		return nil, p.errHere("expected 'd' after count")
	}
	p.pos++

	// sides: digits, '%', or 'F'/'f'.
	term := &termNode{count: count}
	if p.eof() {
		return nil, p.errHere("expected sides specifier")
	}
	switch ch := p.peek(); {
	case isDigit(ch):
		v, col, _ := p.readDigits()
		if v <= 1 {
			return nil, p.errAt(col, "dice sides must be at least 2")
		}
		if v > maxDieSides {
			return nil, p.errAt(col, "dice sides exceeds safety cap")
		}
		term.sides = v
	case ch == '%':
		term.sides = 100
		p.pos++
	case ch == 'F' || ch == 'f':
		term.sides = 3
		term.fudge = true
		p.pos++
	default:
		return nil, p.errHere("expected sides specifier")
	}

	if err := p.parseModifiers(term); err != nil {
		return nil, err
	}
	return term, nil
}

// parseModifiers consumes zero or more post-sides modifiers attached
// to a single term. Stops at '+', '-', whitespace, or end-of-input so
// the outer expression loop can take over.
func (p *parser) parseModifiers(term *termNode) error {
	for !p.eof() {
		ch := p.peek()
		if ch == '+' || ch == '-' || ch == 0 || isSpace(ch) {
			return nil
		}
		switch ch {
		case 'k', 'K':
			if err := p.parseKeep(term); err != nil {
				return err
			}
		case 'd', 'D':
			// After a term, 'd' followed by a digit is the classic
			// "4d6d1" ambiguity. Reject explicitly with column info.
			next := p.peekAt(1)
			if isDigit(next) {
				return p.errHere("ambiguous modifier 'd' — use 'dl' or 'dh'")
			}
			if err := p.parseDrop(term); err != nil {
				return err
			}
		case '!':
			if err := p.parseExplode(term); err != nil {
				return err
			}
		case 'r', 'R':
			if err := p.parseReroll(term); err != nil {
				return err
			}
		default:
			return p.errHere("unexpected character " + quoteByte(ch))
		}
	}
	return nil
}

// parseKeep parses kh<N> or kl<N>.
func (p *parser) parseKeep(term *termNode) error {
	startCol := p.col()
	p.pos++ // consume 'k'
	if p.eof() {
		return p.errAt(startCol, "expected 'h' or 'l' after 'k'")
	}
	var mode keepMode
	switch p.peek() {
	case 'h', 'H':
		mode = keepHigh
	case 'l', 'L':
		mode = keepLow
	default:
		return p.errAt(startCol, "expected 'h' or 'l' after 'k'")
	}
	p.pos++
	v, col, ok := p.readDigits()
	if !ok {
		return p.errAt(col, "expected count after keep modifier")
	}
	if term.keep != keepAll {
		return p.errAt(startCol, "multiple keep/drop modifiers not allowed")
	}
	if v <= 0 || v > term.count {
		return p.errAt(col, "keep count out of range")
	}
	term.keep = mode
	term.keepCount = v
	return nil
}

// parseDrop parses dh<N> or dl<N>.
func (p *parser) parseDrop(term *termNode) error {
	startCol := p.col()
	p.pos++ // consume 'd'
	if p.eof() {
		return p.errAt(startCol, "expected 'h' or 'l' after 'd'")
	}
	var mode keepMode
	switch p.peek() {
	case 'h', 'H':
		mode = dropHigh
	case 'l', 'L':
		mode = dropLow
	default:
		return p.errAt(startCol, "expected 'h' or 'l' after 'd'")
	}
	p.pos++
	v, col, ok := p.readDigits()
	if !ok {
		return p.errAt(col, "expected count after drop modifier")
	}
	if term.keep != keepAll {
		return p.errAt(startCol, "multiple keep/drop modifiers not allowed")
	}
	if v <= 0 || v >= term.count {
		return p.errAt(col, "drop count out of range")
	}
	term.keep = mode
	term.keepCount = v
	return nil
}

// parseExplode parses the '!' modifier with optional comparison and
// value. Rejects unreachable-terminator expressions at Parse time so
// pathological inputs like "1d2!<=2" fail loudly rather than running
// into the runtime depth cap.
func (p *parser) parseExplode(term *termNode) error {
	if term.explode != nil {
		return p.errHere("multiple explode modifiers not allowed")
	}
	startCol := p.col()
	p.pos++ // consume '!'
	op, value, err := p.parseCompareOpt(term.sides)
	if err != nil {
		return err
	}
	spec := &explodeSpec{op: op, value: value}
	if explodeAlwaysTriggers(term.sides, spec) {
		return p.errAt(startCol, "exploding die has no terminating face (all rolls trigger explode)")
	}
	term.explode = spec
	return nil
}

// parseReroll parses the 'r' or 'rr' modifier followed by a mandatory
// comparison or value. Unlike explode, an unreachable reroll condition
// does NOT fail at Parse time — the runtime depth cap terminates the
// loop and surfaces a CapWarning. This mirrors the reality that rr
// with an impossible condition still behaves deterministically: one
// reroll happens, a cap warning fires, Execute returns.
func (p *parser) parseReroll(term *termNode) error {
	if term.reroll != nil {
		return p.errHere("multiple reroll modifiers not allowed")
	}
	p.pos++ // consume 'r'
	once := true
	if !p.eof() && (p.peek() == 'r' || p.peek() == 'R') {
		once = false
		p.pos++
	}
	op, value, err := p.parseCompareRequired(term.sides)
	if err != nil {
		return err
	}
	term.reroll = &rerollSpec{op: op, value: value, once: once}
	return nil
}

// parseCompareOpt parses an optional ">", ">=", "<", "<=" or bare
// number. If nothing is present the returned op is cmpEq and value is
// defaultValue (the die's max face, used for bare "!").
func (p *parser) parseCompareOpt(defaultValue int) (comparisonOp, int, error) {
	if p.eof() {
		return cmpEq, defaultValue, nil
	}
	ch := p.peek()
	if ch == '+' || ch == '-' || ch == 0 || isSpace(ch) {
		return cmpEq, defaultValue, nil
	}
	if !isCompareLead(ch) && !isDigit(ch) {
		// Let the outer loop consume modifiers like 'k'/'d'.
		return cmpEq, defaultValue, nil
	}
	return p.parseCompareRequired(defaultValue)
}

// parseCompareRequired parses a mandatory comparison or bare number.
// Recognises '>', '>=', '<', '<=', and explicit '=' (the cmpEq default
// without any leading operator).
func (p *parser) parseCompareRequired(defaultValue int) (comparisonOp, int, error) {
	op := cmpEq
	if !p.eof() {
		switch p.peek() {
		case '>':
			p.pos++
			if !p.eof() && p.peek() == '=' {
				op = cmpGe
				p.pos++
			} else {
				op = cmpGt
			}
		case '<':
			p.pos++
			if !p.eof() && p.peek() == '=' {
				op = cmpLe
				p.pos++
			} else {
				op = cmpLt
			}
		case '=':
			op = cmpEq
			p.pos++
		}
	}
	v, col, ok := p.readDigits()
	if !ok {
		if op == cmpEq {
			return op, defaultValue, nil
		}
		return op, 0, p.errAt(col, "expected number after comparison")
	}
	return op, v, nil
}

// isCompareLead reports whether ch could begin a comparison operator.
func isCompareLead(ch byte) bool { return ch == '>' || ch == '<' || ch == '=' }

// isSpace reports whether ch is a whitespace byte we reject inside a
// term.
func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// quoteByte formats a single byte for use inside error messages.
func quoteByte(b byte) string {
	if b == 0 {
		return "end-of-input"
	}
	return "'" + string(b) + "'"
}
