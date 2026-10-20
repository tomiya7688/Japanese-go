package transpile

import (
	"bytes"
	"fmt"
	"go/format"
	"go/scanner"
	"go/token"
	"strings"
	"unicode"
)

type lexerState struct {
	blockComment bool
	rawString    bool
}

type item struct {
	text string
	tok  token.Token
}

type compiler struct {
	usedNames map[string]struct{}
	serial    int
}

// Compile converts JGo source into ordinary Go source.
func Compile(src []byte) ([]byte, error) {
	c := &compiler{usedNames: collectIdentifiers(src)}
	lines := strings.Split(string(src), "\n")
	state := lexerState{}
	out := make([]string, 0, len(lines))

	for i, line := range lines {
		converted, err := c.transformLine(line, &state, i+1)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}

	generated := []byte(strings.Join(out, "\n"))
	formatted, err := format.Source(generated)
	if err != nil {
		return nil, fmt.Errorf("generated Go is invalid: %w\n--- generated source ---\n%s", err, generated)
	}
	return formatted, nil
}

func collectIdentifiers(src []byte) map[string]struct{} {
	set := map[string]struct{}{}
	fset := token.NewFileSet()
	file := fset.AddFile("input.jgo", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, scanner.ScanComments)
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.IDENT {
			set[lit] = struct{}{}
		}
	}
	return set
}

func (c *compiler) freshName(base string) string {
	for {
		c.serial++
		name := fmt.Sprintf("__jgo_%s_%d", base, c.serial)
		if _, exists := c.usedNames[name]; !exists {
			c.usedNames[name] = struct{}{}
			return name
		}
	}
}

func (c *compiler) transformLine(line string, state *lexerState, lineNo int) (string, error) {
	indent := line[:len(line)-len(strings.TrimLeftFunc(line, unicode.IsSpace))]
	body := line[len(indent):]

	code, comment, ok := splitTransformable(body, state)
	if !ok {
		return line, nil
	}
	if strings.TrimSpace(code) == "" {
		return line, nil
	}

	toks, err := scanItems(code)
	if err != nil {
		return "", fmt.Errorf("line %d: %w", lineNo, err)
	}
	if len(toks) == 0 {
		return line, nil
	}

	transformed, err := c.transformItems(toks, lineNo)
	if err != nil {
		return "", fmt.Errorf("line %d: %w", lineNo, err)
	}

	result := indent + joinItems(transformed)
	if comment != "" {
		if strings.TrimSpace(result) != "" {
			result += " "
		}
		result += comment
	}
	return result, nil
}

// splitTransformable isolates a trailing // comment. Lines containing block
// comments or multi-line raw strings are deliberately preserved byte-for-byte
// in this first parser so syntax words inside them are never rewritten.
func splitTransformable(line string, state *lexerState) (code, comment string, ok bool) {
	if state.blockComment {
		if idx := strings.Index(line, "*/"); idx >= 0 {
			state.blockComment = false
		}
		return "", "", false
	}
	if state.rawString {
		if strings.Contains(line, "`") {
			state.rawString = false
		}
		return "", "", false
	}

	inDouble := false
	inSingle := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inDouble {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inSingle {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}

		switch ch {
		case '"':
			inDouble = true
		case '\'':
			inSingle = true
		case '`':
			if end := strings.Index(line[i+1:], "`"); end < 0 {
				state.rawString = true
				return "", "", false
			} else {
				i += end + 1
			}
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return line[:i], line[i:], true
			}
			if i+1 < len(line) && line[i+1] == '*' {
				if end := strings.Index(line[i+2:], "*/"); end < 0 {
					state.blockComment = true
				}
				return "", "", false
			}
		}
	}
	return line, "", true
}

func scanItems(code string) ([]item, error) {
	fset := token.NewFileSet()
	file := fset.AddFile("line.jgo", fset.Base(), len(code))
	var firstErr error
	var s scanner.Scanner
	s.Init(file, []byte(code), func(_ token.Position, msg string) {
		if firstErr == nil {
			firstErr = fmt.Errorf("lex error: %s", msg)
		}
	}, 0)

	var out []item
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		text := lit
		if text == "" {
			text = tok.String()
		}
		out = append(out, item{text: text, tok: tok})
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (c *compiler) transformItems(in []item, lineNo int) ([]item, error) {
	if out, ok := transformImport(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformDefer(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformGo(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformContinue(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformFallthrough(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformSelect(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformDefaultCase(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformCase(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformRangeLoop(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformWhileLoop(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := c.transformNumericLoop(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformPostfixSwitch(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformGoto(in); ok {
		return mapKeywords(out), nil
	}
	if out, ok := transformReturn(in); ok {
		return mapKeywords(out), nil
	}

	out := mapKeywords(in)
	out = removeOptionalThen(out)
	return out, nil
}

func transformImport(in []item) ([]item, bool) {
	if len(in) < 2 || in[len(in)-1].text != "読み込む" || in[0].tok != token.STRING {
		return nil, false
	}
	middle := clone(in[1 : len(in)-1])
	middle = removeAllText(middle, "を")
	if len(middle) == 0 {
		return items("import", in[0].text), true
	}
	middle = removeAllText(middle, "として")
	if len(middle) != 1 {
		return nil, false
	}
	return []item{{text: "import", tok: token.IMPORT}, middle[0], in[0]}, true
}

func transformDefer(in []item) ([]item, bool) {
	if len(in) < 2 {
		return nil, false
	}
	switch in[0].text {
	case "関数の終わりに", "関数終わりに", "関数の終わり", "関数終わり":
		return append(items("defer"), clone(in[1:])...), true
	}
	return nil, false
}

func transformGo(in []item) ([]item, bool) {
	if len(in) < 2 {
		return nil, false
	}
	patterns := [][]string{{"を", "並行で実行する"}, {"を", "並行実行する"}, {"並行で実行する"}, {"並行実行する"}, {"を", "並行", "で", "実行する"}, {"を", "並行", "実行する"}, {"並行", "で", "実行する"}, {"並行", "実行する"}}
	for _, p := range patterns {
		if hasSuffix(in, p) {
			expr := clone(in[:len(in)-len(p)])
			if len(expr) > 0 {
				return append(items("go"), expr...), true
			}
		}
	}
	return nil, false
}

func transformContinue(in []item) ([]item, bool) {
	if len(in) == 0 {
		return nil, false
	}
	compact := map[string]bool{"繰り返しを続ける": true, "繰り返し続ける": true}
	if compact[in[0].text] {
		return append(items("continue"), clone(in[1:])...), true
	}
	if len(in) >= 2 && in[0].text == "繰り返し" && in[1].text == "続ける" {
		return append(items("continue"), clone(in[2:])...), true
	}
	if len(in) >= 3 && in[0].text == "繰り返し" && in[1].text == "を" && in[2].text == "続ける" {
		return append(items("continue"), clone(in[3:])...), true
	}
	return nil, false
}

func transformFallthrough(in []item) ([]item, bool) {
	if len(in) == 1 {
		switch in[0].text {
		case "次の分岐へ", "次の場合へ進む", "次の場合進む", "次場合へ進む", "次場合進む":
			return items("fallthrough"), true
		}
	}
	texts := itemTexts(in)
	patterns := [][]string{{"次", "の", "場合", "へ", "進む"}, {"次", "の", "場合", "進む"}, {"次", "場合", "へ", "進む"}, {"次", "場合", "進む"}, {"次", "の場合", "へ", "進む"}, {"次", "の場合", "進む"}}
	for _, p := range patterns {
		if equalStrings(texts, p) {
			return items("fallthrough"), true
		}
	}
	return nil, false
}

func transformSelect(in []item) ([]item, bool) {
	if len(in) < 2 || in[len(in)-1].text != "{" {
		return nil, false
	}
	prefix := itemTexts(in[:len(in)-1])
	if equalStrings(prefix, []string{"通信を選ぶ"}) || equalStrings(prefix, []string{"通信選ぶ"}) || equalStrings(prefix, []string{"通信", "を", "選ぶ"}) || equalStrings(prefix, []string{"通信", "選ぶ"}) {
		return items("select", "{"), true
	}
	return nil, false
}

func transformDefaultCase(in []item) ([]item, bool) {
	if len(in) < 2 || in[len(in)-1].text != ":" {
		return nil, false
	}
	prefix := itemTexts(in[:len(in)-1])
	forms := [][]string{{"それ以外の場合"}, {"それ以外場合"}, {"それ以外", "の場合"}, {"それ以外", "場合"}, {"それ以外", "の", "場合"}}
	for _, f := range forms {
		if equalStrings(prefix, f) {
			return items("default", ":"), true
		}
	}
	return nil, false
}

func transformCase(in []item) ([]item, bool) {
	if len(in) < 3 || in[len(in)-1].text != ":" {
		return nil, false
	}
	markerStart := -1
	for i := len(in) - 2; i >= 0; i-- {
		if in[i].text == "の場合" || in[i].text == "場合" {
			markerStart = i
			if i > 0 && in[i].text == "場合" && in[i-1].text == "の" {
				markerStart = i - 1
			}
			break
		}
	}
	if markerStart <= 0 || markerStart > len(in)-2 {
		return nil, false
	}
	if markerStart != len(in)-2 && !(markerStart == len(in)-3 && in[markerStart].text == "の" && in[markerStart+1].text == "場合") {
		return nil, false
	}
	expr := clone(in[:markerStart])
	return append(append(items("case"), expr...), item{text: ":", tok: token.COLON}), true
}

func transformRangeLoop(in []item) ([]item, bool) {
	if len(in) < 5 || in[len(in)-1].text != "{" {
		return nil, false
	}
	loopIdx := len(in) - 2
	if in[loopIdx].text != "繰り返し" {
		return nil, false
	}
	rangeIdx := loopIdx - 1
	if rangeIdx < 1 || (in[rangeIdx].text != "列挙して" && in[rangeIdx].text != "列挙") {
		return nil, false
	}
	body := clone(in[:rangeIdx])
	if len(body) > 0 && body[len(body)-1].text == "を" {
		body = body[:len(body)-1]
	}
	if len(body) < 2 {
		return nil, false
	}

	from := indexTopLevel(body, "から")
	var target, vars []item
	if from >= 0 {
		target = clone(body[:from])
		vars = clone(body[from+1:])
	} else {
		var n int
		target, n = takeOneExpr(body)
		if n == 0 {
			return nil, false
		}
		vars = clone(body[n:])
	}
	if len(target) == 0 || len(vars) == 0 {
		return nil, false
	}
	out := items("for")
	out = append(out, vars...)
	out = append(out, items(":=", "range")...)
	out = append(out, target...)
	out = append(out, item{text: "{", tok: token.LBRACE})
	return out, true
}

func transformWhileLoop(in []item) ([]item, bool) {
	if len(in) < 4 || in[len(in)-1].text != "{" || in[len(in)-2].text != "繰り返し" {
		return nil, false
	}
	end := len(in) - 2
	marker := -1
	if end >= 1 && (in[end-1].text == "の間" || in[end-1].text == "間") {
		marker = end - 1
	} else if end >= 2 && in[end-2].text == "の" && in[end-1].text == "間" {
		marker = end - 2
	}
	if marker <= 0 {
		return nil, false
	}
	cond := clone(in[:marker])
	out := append(items("for"), cond...)
	out = append(out, item{text: "{", tok: token.LBRACE})
	return out, true
}

func (c *compiler) transformNumericLoop(in []item) ([]item, bool) {
	if len(in) < 5 || in[0].text != "繰り返し" || in[len(in)-1].text != "{" {
		return nil, false
	}
	body := clone(in[1 : len(in)-1])
	if len(body) < 3 || body[0].tok != token.IDENT {
		return nil, false
	}
	name := body[0]
	body = body[1:]
	if len(body) > 0 && body[0].text == "が" {
		body = body[1:]
	}
	if len(body) < 2 {
		return nil, false
	}

	var start, end []item
	from := indexTopLevel(body, "から")
	if from >= 0 {
		start = clone(body[:from])
		rest := clone(body[from+1:])
		if len(rest) > 0 && rest[len(rest)-1].text == "まで" {
			rest = rest[:len(rest)-1]
		}
		end = rest
	} else {
		var n int
		start, n = takeOneExpr(body)
		if n == 0 {
			return nil, false
		}
		rest := clone(body[n:])
		if len(rest) > 0 && rest[len(rest)-1].text == "まで" {
			rest = rest[:len(rest)-1]
		}
		var m int
		end, m = takeOneExpr(rest)
		if m == 0 || m != len(rest) {
			return nil, false
		}
	}
	if len(start) == 0 || len(end) == 0 {
		return nil, false
	}

	endName := c.freshName("end")
	doneName := c.freshName("done")
	text := fmt.Sprintf("for %s, %s, %s := %s, %s, false; !%s && %s <= %s; func() { if %s == %s { %s = true } else { %s++ } }() {",
		name.text, endName, doneName, joinItems(start), joinItems(end), doneName, name.text, endName, name.text, endName, doneName, name.text)
	out, err := scanItems(text)
	if err != nil {
		return nil, false
	}
	return out, true
}

func transformPostfixSwitch(in []item) ([]item, bool) {
	if len(in) < 3 || in[len(in)-1].text != "{" || in[len(in)-2].text != "分岐" {
		return nil, false
	}
	expr := clone(in[:len(in)-2])
	if len(expr) > 0 && expr[len(expr)-1].text == "で" {
		expr = expr[:len(expr)-1]
	}
	if len(expr) == 0 {
		return nil, false
	}
	out := append(items("switch"), expr...)
	out = append(out, item{text: "{", tok: token.LBRACE})
	return out, true
}

func transformGoto(in []item) ([]item, bool) {
	if len(in) < 2 || in[len(in)-1].text != "飛ぶ" || in[0].text == "飛ぶ" {
		return nil, false
	}
	label := clone(in[:len(in)-1])
	if len(label) > 0 && label[len(label)-1].text == "へ" {
		label = label[:len(label)-1]
	}
	if len(label) != 1 {
		return nil, false
	}
	return append(items("goto"), label...), true
}

func transformReturn(in []item) ([]item, bool) {
	if len(in) < 2 || in[len(in)-1].text != "返す" || in[0].text == "返す" {
		return nil, false
	}
	expr := clone(in[:len(in)-1])
	if len(expr) > 0 && expr[len(expr)-1].text == "を" {
		expr = expr[:len(expr)-1]
	}
	if len(expr) == 0 {
		return items("return"), true
	}
	return append(items("return"), expr...), true
}

func mapKeywords(in []item) []item {
	out := clone(in)
	mapping := map[string]string{
		"パッケージ":    "package",
		"読み込む":     "import",
		"関数":       "func",
		"変数":       "var",
		"定数":       "const",
		"型":        "type",
		"構造体":      "struct",
		"インターフェース": "interface",
		"マップ":      "map",
		"チャネル":     "chan",
		"もし":       "if",
		"それ以外":     "else",
		"繰り返し":     "for",
		"列挙":       "range",
		"抜ける":      "break",
		"分岐":       "switch",
		"場合":       "case",
		"既定":       "default",
		"選ぶ":       "select",
		"行け":       "go",
		"返す":       "return",
		"飛ぶ":       "goto",
		"次の分岐へ":    "fallthrough",
		"関数の終わりに":  "defer",
		"関数終わりに":   "defer",
		"関数の終わり":   "defer",
		"関数終わり":    "defer",
		"繰り返しを続ける": "continue",
		"繰り返し続ける":  "continue",
		"通信を選ぶ":    "select",
		"通信選ぶ":     "select",
	}
	for i := range out {
		if repl, ok := mapping[out[i].text]; ok {
			out[i].text = repl
		}
	}
	return out
}

func removeOptionalThen(in []item) []item {
	out := make([]item, 0, len(in))
	for i, it := range in {
		if it.text == "ならば" {
			if i+1 < len(in) && in[i+1].text == "{" {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func items(texts ...string) []item {
	out := make([]item, 0, len(texts))
	for _, t := range texts {
		out = append(out, item{text: t})
	}
	return out
}

func clone(in []item) []item {
	return append([]item(nil), in...)
}

func itemTexts(in []item) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = in[i].text
	}
	return out
}

func joinItems(in []item) string {
	var b bytes.Buffer
	for i, it := range in {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(it.text)
	}
	return b.String()
}

func removeAllText(in []item, text string) []item {
	out := in[:0]
	for _, it := range in {
		if it.text != text {
			out = append(out, it)
		}
	}
	return out
}

func hasSuffix(in []item, suffix []string) bool {
	if len(suffix) > len(in) {
		return false
	}
	start := len(in) - len(suffix)
	for i := range suffix {
		if in[start+i].text != suffix[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func indexTopLevel(in []item, text string) int {
	depth := 0
	for i, it := range in {
		switch it.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && it.text == text {
				return i
			}
		}
	}
	return -1
}

func takeOneExpr(in []item) ([]item, int) {
	if len(in) == 0 {
		return nil, 0
	}
	if in[0].text != "(" {
		return clone(in[:1]), 1
	}
	depth := 0
	for i, it := range in {
		switch it.text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return clone(in[:i+1]), i + 1
			}
		}
	}
	return nil, 0
}
