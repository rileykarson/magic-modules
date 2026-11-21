package gopatcher

import (
	"fmt"
	"strconv"
	"strings"
)

type tmplAction struct {
	start int
	end   int
	inner string
}

// condBlockSpan tracks the [start, end) byte span of a {{ if ... }} ... {{ end }}
// block in a .go.tmpl file so field insertions stay in the composite literal's own scope.
type condBlockSpan struct {
	id    int
	start int
	end   int
}

type tmplInfo struct {
	actions    []tmplAction
	condBlocks []condBlockSpan
}

func parseTemplateStructure(src []byte) tmplInfo {
	var actions []tmplAction
	n := len(src)
	for i := 0; i < n; {
		if i+1 < n && src[i] == '{' && src[i+1] == '{' {
			start := i
			i += 2
			if i+1 < n && src[i] == '/' && src[i+1] == '*' {
				i += 2
				for i+3 < n {
					if src[i] == '*' && src[i+1] == '/' && src[i+2] == '}' && src[i+3] == '}' {
						i += 4
						break
					}
					i++
				}
			} else {
				inDouble, inRaw, inChar := false, false, false
				for i < n {
					c := src[i]
					if inDouble {
						if c == '\\' && i+1 < n {
							i += 2
							continue
						}
						if c == '"' {
							inDouble = false
						}
						i++
						continue
					}
					if inRaw {
						if c == '`' {
							inRaw = false
						}
						i++
						continue
					}
					if inChar {
						if c == '\\' && i+1 < n {
							i += 2
							continue
						}
						if c == '\'' {
							inChar = false
						}
						i++
						continue
					}
					switch c {
					case '"':
						inDouble = true
					case '`':
						inRaw = true
					case '\'':
						inChar = true
					case '}':
						if i+1 < n && src[i+1] == '}' {
							i += 2
							goto foundEnd
						}
					}
					i++
				}
			}
		foundEnd:
			end := min(i, n)
			inner := ""
			if end-start >= 4 {
				inner = string(src[start+2 : end-2])
				inner = strings.TrimPrefix(inner, "-")
				inner = strings.TrimSuffix(inner, "-")
				inner = strings.TrimSpace(inner)
			}
			actions = append(actions, tmplAction{
				start: start,
				end:   end,
				inner: inner,
			})
			continue
		}
		i++
	}

	var condBlocks []condBlockSpan
	type openBlock struct {
		id    int
		start int
	}
	var stack []openBlock
	nextID := 1
	for _, act := range actions {
		if isBlockOpener(act.inner) {
			stack = append(stack, openBlock{id: nextID, start: act.start})
			nextID++
		} else if act.inner == "end" && len(stack) > 0 {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			condBlocks = append(condBlocks, condBlockSpan{
				id:    top.id,
				start: top.start,
				end:   act.end,
			})
		}
	}

	return tmplInfo{
		actions:    actions,
		condBlocks: condBlocks,
	}
}

func isBlockOpener(inner string) bool {
	for _, kw := range []string{"if ", "if(", "range ", "with ", "define ", "block "} {
		if strings.HasPrefix(inner, kw) {
			return true
		}
	}
	return false
}

// condScopeKey returns a canonical key encoding which template conditional blocks enclose offset.
func (ti tmplInfo) condScopeKey(offset int) string {
	if len(ti.condBlocks) == 0 {
		return ""
	}
	var b strings.Builder
	for _, cb := range ti.condBlocks {
		if offset >= cb.start && offset < cb.end {
			fmt.Fprintf(&b, "%d,", cb.id)
		}
	}
	return b.String()
}

// renderMaskedGoTemplate produces a 1:1 byte-length Go source slice from a .go.tmpl buffer
// by evaluating template conditionals for targetVersion ("beta" or "ga"), masking inactive
// branches and template directives with spaces while preserving every '\n' byte at its exact offset.
func renderMaskedGoTemplate(src []byte, info tmplInfo, targetVersion string) []byte {
	out := make([]byte, len(src))
	copy(out, src)

	blankRange := func(a, b int) {
		for idx := a; idx < b && idx < len(out); idx++ {
			if out[idx] != '\n' {
				out[idx] = ' '
			}
		}
	}

	type condFrame struct {
		parentActive bool
		branchActive bool
		matchedAny   bool
	}
	var stack []condFrame
	isActive := func() bool {
		if len(stack) == 0 {
			return true
		}
		top := stack[len(stack)-1]
		return top.parentActive && top.branchActive
	}

	cursor := 0
	for _, act := range info.actions {
		if !isActive() {
			blankRange(cursor, act.start)
		}
		inner := act.inner
		switch {
		case strings.HasPrefix(inner, "/*"):
			blankRange(act.start, act.end)
		case strings.HasPrefix(inner, "if ") || strings.HasPrefix(inner, "if("):
			condExpr := strings.TrimSpace(strings.TrimPrefix(inner, "if"))
			pActive := isActive()
			cond := pActive && evalTemplateCond(condExpr, targetVersion)
			stack = append(stack, condFrame{
				parentActive: pActive,
				branchActive: cond,
				matchedAny:   cond,
			})
			blankRange(act.start, act.end)
		case strings.HasPrefix(inner, "range "), strings.HasPrefix(inner, "with "), strings.HasPrefix(inner, "define "), strings.HasPrefix(inner, "block "):
			pActive := isActive()
			stack = append(stack, condFrame{
				parentActive: pActive,
				branchActive: pActive,
				matchedAny:   pActive,
			})
			blankRange(act.start, act.end)
		case strings.HasPrefix(inner, "else if ") || strings.HasPrefix(inner, "else if("):
			condExpr := strings.TrimSpace(strings.TrimPrefix(inner, "else if"))
			if len(stack) > 0 {
				top := &stack[len(stack)-1]
				if !top.parentActive || top.matchedAny {
					top.branchActive = false
				} else {
					cond := evalTemplateCond(condExpr, targetVersion)
					top.branchActive = cond
					if cond {
						top.matchedAny = true
					}
				}
			}
			blankRange(act.start, act.end)
		case inner == "else":
			if len(stack) > 0 {
				top := &stack[len(stack)-1]
				if !top.parentActive || top.matchedAny {
					top.branchActive = false
				} else {
					top.branchActive = true
					top.matchedAny = true
				}
			}
			blankRange(act.start, act.end)
		case inner == "end":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			blankRange(act.start, act.end)
		case isActive() && (strings.HasPrefix(inner, "\"") || strings.HasPrefix(inner, "`")):
			blankRange(act.start, act.end)
			if unq, err := strconv.Unquote(inner); err == nil && len(unq) <= act.end-act.start {
				copy(out[act.start:], []byte(unq))
			}
		default:
			blankRange(act.start, act.end)
		}
		cursor = act.end
	}
	if !isActive() {
		blankRange(cursor, len(out))
	}
	return out
}

func evalTemplateCond(expr string, targetVersion string) bool {
	expr = strings.TrimSpace(expr)
	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		depth := 0
		balanced := true
		for idx, ch := range expr {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 && idx < len(expr)-1 {
					balanced = false
					break
				}
			}
		}
		if balanced {
			expr = strings.TrimSpace(expr[1 : len(expr)-1])
		} else {
			break
		}
	}

	if strings.HasPrefix(expr, "not ") || strings.HasPrefix(expr, "not(") {
		return !evalTemplateCond(strings.TrimSpace(expr[3:]), targetVersion)
	}
	if strings.HasPrefix(expr, "or ") || strings.HasPrefix(expr, "or(") {
		for _, a := range splitTemplateArgs(strings.TrimSpace(expr[2:])) {
			if evalTemplateCond(a, targetVersion) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(expr, "and ") || strings.HasPrefix(expr, "and(") {
		for _, a := range splitTemplateArgs(strings.TrimSpace(expr[3:])) {
			if !evalTemplateCond(a, targetVersion) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(expr, "eq ") || strings.HasPrefix(expr, "eq(") {
		args := splitTemplateArgs(strings.TrimSpace(expr[2:]))
		if len(args) >= 2 {
			v0 := evalTemplateAtom(args[0], targetVersion)
			for _, other := range args[1:] {
				if v0 == evalTemplateAtom(other, targetVersion) {
					return true
				}
			}
			return false
		}
	}
	if strings.HasPrefix(expr, "ne ") || strings.HasPrefix(expr, "ne(") {
		args := splitTemplateArgs(strings.TrimSpace(expr[2:]))
		if len(args) == 2 {
			return evalTemplateAtom(args[0], targetVersion) != evalTemplateAtom(args[1], targetVersion)
		}
	}
	return true
}

func evalTemplateAtom(a string, targetVersion string) string {
	a = strings.TrimSpace(a)
	switch a {
	case "$.TargetVersionName", ".TargetVersionName":
		return targetVersion
	case "$.Compiler", ".Compiler":
		return ""
	}
	if u, err := strconv.Unquote(a); err == nil {
		return u
	}
	return a
}

func splitTemplateArgs(s string) []string {
	var args []string
	s = strings.TrimSpace(s)
	n := len(s)
	for i := 0; i < n; {
		for i < n && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= n {
			break
		}
		start := i
		if s[i] == '(' {
			depth := 0
			for i < n {
				if s[i] == '(' {
					depth++
				} else if s[i] == ')' {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				i++
			}
			args = append(args, s[start:i])
		} else if s[i] == '"' || s[i] == '`' {
			q := s[i]
			i++
			for i < n {
				if q == '"' && s[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if s[i] == q {
					i++
					break
				}
				i++
			}
			args = append(args, s[start:i])
		} else {
			for i < n && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
				i++
			}
			args = append(args, s[start:i])
		}
	}
	return args
}

// unquoteGoLiteralInSource extracts the runtime Go string value from raw Go/template bytes
// spanning [start, end) of a *ast.BasicLit (token.STRING).
// For .go.tmpl files, pure string-literal template actions like {{"{{"}} and {{"}}"}} are
// evaluated first; if a non-literal template directive (e.g. {{ if ... }}) appears inside the
// string literal, ok=false is returned so callers treat it as non-literal.
func unquoteGoLiteralInSource(origBytes []byte, start, end int, isTmpl bool) (string, bool) {
	if start < 0 || end > len(origBytes) || start >= end {
		return "", false
	}
	raw := origBytes[start:end]
	if !isTmpl || !strings.Contains(string(raw), "{{") {
		val, err := strconv.Unquote(string(raw))
		if err != nil {
			return "", false
		}
		return val, true
	}

	info := parseTemplateStructure(raw)
	var rendered strings.Builder
	cursor := 0
	for _, act := range info.actions {
		rendered.Write(raw[cursor:act.start])
		inner := act.inner
		if strings.HasPrefix(inner, "\"") || strings.HasPrefix(inner, "`") {
			unq, err := strconv.Unquote(inner)
			if err != nil {
				return "", false
			}
			rendered.WriteString(unq)
		} else {
			// Non-literal template directive inside string literal: do not overwrite.
			return "", false
		}
		cursor = act.end
	}
	rendered.Write(raw[cursor:])
	val, err := strconv.Unquote(rendered.String())
	if err != nil {
		return "", false
	}
	return val, true
}

// escapeLiteralForGoTemplate escapes {{ and }} inside an already-formatted Go string literal
// ("..." or `...`) so Go's text/template engine renders the exact literal braces without error.
func escapeLiteralForGoTemplate(goLiteral string) string {
	if len(goLiteral) < 2 {
		return goLiteral
	}
	if !strings.Contains(goLiteral, "{{") && !strings.Contains(goLiteral, "}}") {
		return goLiteral
	}
	openQuote := goLiteral[:1]
	closeQuote := goLiteral[len(goLiteral)-1:]
	body := goLiteral[1 : len(goLiteral)-1]

	// Replace in a single pass so {{"{{"}} replacements do not get re-replaced.
	var b strings.Builder
	b.Grow(len(goLiteral) + 16)
	b.WriteString(openQuote)
	for i := 0; i < len(body); {
		if i+1 < len(body) && body[i] == '{' && body[i+1] == '{' {
			b.WriteString(`{{"{{"}}`)
			i += 2
			continue
		}
		if i+1 < len(body) && body[i] == '}' && body[i+1] == '}' {
			b.WriteString(`{{"}}"}}`)
			i += 2
			continue
		}
		b.WriteByte(body[i])
		i++
	}
	b.WriteString(closeQuote)
	return b.String()
}
