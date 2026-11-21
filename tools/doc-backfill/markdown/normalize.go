package markdown

import (
	"regexp"
	"strings"
)

var (
	// Standalone bracketed badge at start of description:
	//   [Beta](../guides/provider_versions.html.markdown)
	//   [Beta]
	//   [Write-only](...)
	leadingBracketBadgeRe = regexp.MustCompile(`(?i)^\[(beta|write-only|write only|deprecated)\](?:\([^)\n]+\))?`)

	// Malformed unclosed leading badge with [Beta](...) on its own line, e.g. in compute_instance.html.markdown:
	//   (Optional, [Beta](../guides/provider_versions.html.markdown)\n
	unclosedBetaBadgeRe = regexp.MustCompile(`(?i)^\([ \t]*(optional|required)(?:[ \t]*,[ \t]*|[ \t]+)\[beta\]\([^)\n]+\)[ \t]*\n`)

	// Malformed unclosed `(Optional, Configures ...` typo in compute_instance_template.html.markdown:
	unclosedCommaBadgeRe = regexp.MustCompile(`^\([ \t]*(Optional|Required)[ \t]*,[ \t]+([A-Z][a-z]+)`)

	// Recognized keywords that identify a parenthesized group as a schema metadata badge
	// rather than prose like `(Default 'SURGE')` or `(e.g. foo)`.
	badgeStartKeywordRe = regexp.MustCompile(`(?i)^(?:required|optional|output|computed|input[- ]only|read[- ]only|write[- ]only|beta|deprecated|\[beta\]|\[write[- ]only\]|\.\./guides/provider_versions\.html)`)

	// Structural pointer patterns used both to verify (#anchor) ownership and to strip boilerplate sentences.
	structuralAnchorCheckRe = regexp.MustCompile(`(?is)(?:\bstructure\b[^.\n]{0,80}\[[a-z \t\n]+(?:below|above)\]\(#([a-zA-Z0-9_-]+)\)|\bsee[ \t\n]+(?:the[ \t\n]+)?\[(?:below[ \t\n]+for[ \t\n]+nested[ \t\n]+schema|structure[ \t\n]+below|section[ \t\n]+below)\]\(#([a-zA-Z0-9_-]+)\)|\bconfiguration[ \t\n]+option[ \t\n]+are[ \t\n]+\[[a-z \t\n]+below\]\(#([a-zA-Z0-9_-]+)\)|(?:^|\.[ \t\n]+)\[documented[ \t\n]+(?:below|above)\]\(#([a-zA-Z0-9_-]+)\))`)

	// Structural nested-block pointer sentences to strip from CleanDescription:
	// 1. "(; |, )?(The <x> )?Structure (is )?(documented below )?[documented|defined|detailed below|above](#anchor).?"
	// Note: `(?-i:\bThe)` is case-sensitive and unbackticked `<x>` excludes `.` so preceding sentence endings
	// like `...of the pool. Structure is [documented below]...` are never consumed!
	stripStructureLinkRe = regexp.MustCompile("(?i)(?:([;,])[ \\t]*|(?-i:\\bThe)[ \\t]+(?:`[a-zA-Z0-9_.\\-]+`|[a-zA-Z0-9_\\-]+)[ \\t]+)?\\b[Ss]tructure\\b(?:[ \\t\\n]+is)?(?:[ \\t\\n]+documented[ \\t\\n]+(?:below|above))?[ \\t\\n]+\\[[a-zA-Z \\t\\n]+(?:below|above)\\]\\(#[^)\\s]+\\)([.:])?")

	// 2. "See [below for nested schema](#anchor).?" / "See [structure below](#anchor).?" / "See the [section below](#anchor) for details on configuration."
	stripSeeBelowNestedRe = regexp.MustCompile(`(?i)\bSee[ \t\n]+(?:the[ \t\n]+)?\[(?:below[ \t\n]+for[ \t\n]+nested[ \t\n]+schema|structure[ \t\n]+below|section[ \t\n]+below)\]\(#[^)\s]+\)(?:[ \t\n]+for[ \t\n]+details[ \t\n]+on[ \t\n]+configuration)?[.:]?`)

	// 3. "More details about this configuration option are [detailed below](#anchor).?"
	stripMoreDetailsBelowRe = regexp.MustCompile(`(?i)\bMore[ \t\n]+details[ \t\n]+about[ \t\n]+this[ \t\n]+configuration[ \t\n]+option[ \t\n]+are[ \t\n]+\[[a-zA-Z \t\n]+below\]\(#[^)\s]+\)[.:]?`)

	// 4. Standalone sentence "[documented below](#anchor)." after sentence period or at start of description.
	stripBareDocBelowLinkRe = regexp.MustCompile(`(?i)(^|\.)[ \t\n]*\[documented[ \t\n]+(?:below|above)\]\(#[^)\s]+\)[.:]?`)

	// 5. Unlinked "(; |, )?(The <x> )?Structure (is )?(documented|defined|detailed|described) (below|above).?"
	stripStructurePlainRe = regexp.MustCompile("(?i)(?:([;,])[ \\t]*|(?-i:\\bThe)[ \\t]+(?:`[a-zA-Z0-9_.\\-]+`|[a-zA-Z0-9_\\-]+)[ \\t]+)?\\b[Ss]tructure\\b(?:[ \\t\\n]+is)?[ \\t\\n]+(?:documented|defined|detailed|described)[ \\t\\n]+(?:below|above)([.:])?")

	// 6. "The configuration is (detailed|documented|defined|described) below.?"
	stripConfigDetailedBelowRe = regexp.MustCompile(`(?i)\bThe[ \t\n]+configuration[ \t\n]+is[ \t\n]+(?:detailed|documented|defined|described)[ \t\n]+below[.:]?`)

	// 7. Trailing inline block header introducer left on parent field after indented child bullets were split out
	// (e.g. `... The `writable_cgroups` block supports:` in container_cluster.html.markdown).
	stripTrailingInlineBlockHeaderRe = regexp.MustCompile("(?i)(?:^|[ \\t\\n]+)(?-i:\\bThe)[ \\t]+(?:(?:optional|required|computed|read[- ]only)[,\\s]+)*`[a-zA-Z0-9_.\\[\\]-]+`(?:[ \\t]+(?:block|blocks|subblock|subblocks|sublist|sublists|list|object|structure))?[ \\t]+(?:supports|support|contains|holds):[ \\t]*$")
)

func hasUnlinkedStructuralPointer(rawDesc string) bool {
	lower := strings.ToLower(rawDesc)
	if !strings.Contains(lower, "below") && !strings.Contains(lower, "above") {
		return false
	}
	return stripStructurePlainRe.MatchString(rawDesc) || stripConfigDetailedBelowRe.MatchString(rawDesc)
}

func hasStructuralPointerToAnchor(rawDesc, anchor string) bool {
	for _, m := range structuralAnchorCheckRe.FindAllStringSubmatch(rawDesc, -1) {
		for _, grp := range m[1:] {
			if grp == anchor {
				return true
			}
		}
	}
	return false
}

func buildFieldDoc(leafName, fullPath, sectionKind, rawDesc string) *FieldDoc {
	fd := &FieldDoc{
		Name:           leafName,
		Path:           fullPath,
		Section:        sectionKind,
		RawDescription: strings.TrimSpace(rawDesc),
	}
	if sectionKind == "attribute" {
		fd.Output = true
	}

	// Step 1: Dedent continuation lines relative to the field bullet BEFORE stripping
	// leading multiline badges, so multiline prose indentation is cleanly normalized.
	dedented := dedentFieldContinuation(rawDesc)

	// Step 2: Strip leading metadata badges and populate FieldDoc boolean flags.
	withoutBadges := stripLeadingBadges(dedented, fd)

	// Also detect write-only note / Output only prefix if not already set by badges.
	lowerClean := strings.ToLower(withoutBadges)
	if strings.Contains(lowerClean, "this property is write-only") {
		fd.WriteOnly = true
	}
	if strings.HasPrefix(withoutBadges, "Output only.") {
		fd.Output = true
	}

	// Step 3: Strip structural nested-block pointer sentences cleanly.
	withoutPointers := stripStructuralPointerSentences(withoutBadges)

	// Step 4: Final per-line right-trim and blank-line normalization.
	lines := strings.Split(withoutPointers, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	fd.CleanDescription = collapseExcessBlankLines(trimBlankLines(lines))
	return fd
}

func dedentFieldContinuation(raw string) string {
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return ""
	}

	firstLineInline := strings.TrimSpace(lines[0]) != ""

	// Find base continuation indent from the initial indented block under the bullet
	// (ignoring fenced code blocks' internal lines).
	bulletIndent := detectBulletIndent(lines, firstLineInline)
	if bulletIndent <= 0 {
		out := make([]string, len(lines))
		inFence := false
		for i, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = !inFence
			}
			if i == 0 && firstLineInline {
				out[i] = strings.TrimSpace(l)
			} else if inFence {
				out[i] = strings.TrimRight(l, " \t")
			} else {
				out[i] = strings.TrimRight(expandLeadingTabs(l), " \t")
			}
		}
		return trimBlankLines(out)
	}

	out := make([]string, len(lines))
	inFence := false
	fenceDedent := 0

	for i, l := range lines {
		if i == 0 && firstLineInline {
			out[i] = strings.TrimSpace(l)
			continue
		}
		if strings.TrimSpace(l) == "" {
			out[i] = ""
			continue
		}
		expanded := expandLeadingTabs(l)
		trimmed := strings.TrimSpace(expanded)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				ind := countLeadingSpaces(expanded)
				if ind >= bulletIndent {
					fenceDedent = bulletIndent
				} else {
					fenceDedent = ind
				}
			} else {
				inFence = false
			}
			out[i] = strings.TrimRight(stripNSpaces(expanded, fenceDedent), " \t")
			continue
		}
		if inFence {
			out[i] = strings.TrimRight(stripNSpaces(expanded, fenceDedent), " \t")
			continue
		}

		ind := countLeadingSpaces(expanded)
		if ind >= bulletIndent {
			out[i] = strings.TrimRight(expanded[bulletIndent:], " \t")
		} else {
			// Paragraph or callout written at column 0 (e.g. unindented **Note**: or -> callout):
			// preserve as-is without eating non-space characters.
			out[i] = strings.TrimRight(expanded, " \t")
		}
	}
	return trimBlankLines(out)
}

func detectBulletIndent(lines []string, firstLineInline bool) int {
	startIdx := 0
	if firstLineInline {
		startIdx = 1
	}
	minIndent := -1
	inFence := false

	for i := startIdx; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			// Stop measuring the initial bullet indent once we have seen at least one indented
			// continuation line and hit a paragraph break followed by a column-0 line.
			if minIndent > 0 && i+1 < len(lines) {
				// Peek next non-empty line.
				for j := i + 1; j < len(lines); j++ {
					if strings.TrimSpace(lines[j]) == "" {
						continue
					}
					if countLeadingSpacesExpanded(lines[j]) == 0 {
						return minIndent
					}
					break
				}
			}
			continue
		}
		expanded := expandLeadingTabs(l)
		trimmed := strings.TrimSpace(expanded)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			ind := countLeadingSpaces(expanded)
			if ind > 0 && (minIndent == -1 || ind < minIndent) {
				minIndent = ind
			}
			continue
		}
		if inFence {
			continue
		}
		ind := countLeadingSpaces(expanded)
		if ind == 0 {
			if minIndent == -1 {
				return 0
			}
			return minIndent
		}
		if minIndent == -1 || ind < minIndent {
			minIndent = ind
		}
	}
	if minIndent < 0 {
		return 0
	}
	return minIndent
}

func stripLeadingBadges(text string, fd *FieldDoc) string {
	s := text
	strippedAny := false
	for {
		s = strings.TrimLeft(s, " \t\n")
		if s == "" {
			return ""
		}

		// Strip stray trailing `,` or `.` immediately following a stripped badge
		// (e.g. `(Optional), Specifies...` in compute_instance_group_manager or `(Optional).` in container_cluster).
		if strippedAny && (s[0] == ',' || s[0] == '.') && (len(s) == 1 || s[1] == ' ' || s[1] == '\t' || s[1] == '\n') {
			s = s[1:]
			continue
		}

		// Strip stray leading separator `- ` / `-- ` / `: ` (e.g. `* `foo` (Optional) - Enable ...`).
		if strings.HasPrefix(s, "-- ") || strings.HasPrefix(s, "--\t") || strings.HasPrefix(s, "--\n") {
			s = strings.TrimLeft(s[2:], " \t")
			continue
		}
		if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "-\t") || strings.HasPrefix(s, "-\n") {
			s = strings.TrimLeft(s[1:], " \t")
			continue
		}
		if strings.HasPrefix(s, ": ") || strings.HasPrefix(s, ":\t") {
			s = strings.TrimLeft(s[1:], " \t")
			continue
		}

		// 1. Standalone leading bracket badge: [Beta](...), [Beta], [Write-only](...)
		if m := leadingBracketBadgeRe.FindStringIndex(s); m != nil {
			applyBadgeFlags(s[:m[1]], fd)
			s = s[m[1]:]
			strippedAny = true
			continue
		}

		// 2. Unclosed `(Optional, [Beta](...)\n` typo
		if m := unclosedBetaBadgeRe.FindStringIndex(s); m != nil {
			applyBadgeFlags(s[:m[1]], fd)
			s = s[m[1]:]
			strippedAny = true
			continue
		}

		// 3. Balanced leading `(...)` badge
		if s[0] == '(' {
			if endIdx, ok := matchBalancedBadgeParen(s); ok {
				inner := strings.TrimSpace(s[1 : endIdx-1])
				if badgeStartKeywordRe.MatchString(inner) {
					applyBadgeFlags(inner, fd)
					s = s[endIdx:]
					strippedAny = true
					continue
				}
			} else if m := unclosedCommaBadgeRe.FindStringSubmatchIndex(s); m != nil {
				// Typo like `(Optional, Configures network performance...` where `)` was omitted.
				// Strip `(Optional, ` and keep `Configures...`.
				applyBadgeFlags(s[m[2]:m[3]], fd)
				s = s[m[4]:]
				strippedAny = true
				continue
			}
		}

		break
	}

	// Re-dedent if stripping a first-line badge left uniform indentation on the remaining lines.
	lines := strings.Split(s, "\n")
	if len(lines) > 1 && strings.TrimSpace(lines[0]) != "" && countLeadingSpacesExpanded(lines[0]) > 0 {
		return dedentYAMLBlock(lines)
	}
	return s
}

func matchBalancedBadgeParen(s string) (end int, ok bool) {
	depth := 0
	limit := len(s)
	if limit > 240 {
		limit = 240
	}
	newlines := 0
	for i := 0; i < limit; i++ {
		switch s[i] {
		case '\n':
			newlines++
			if newlines > 2 {
				return 0, false
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func applyBadgeFlags(badge string, fd *FieldDoc) {
	lower := strings.ToLower(badge)
	if strings.Contains(lower, "required") {
		fd.Required = true
	}
	if strings.Contains(lower, "optional") {
		fd.Optional = true
	}
	if strings.Contains(lower, "output") || strings.Contains(lower, "computed") ||
		strings.Contains(lower, "read only") || strings.Contains(lower, "read-only") {
		fd.Output = true
	}
	if strings.Contains(lower, "write-only") || strings.Contains(lower, "write only") {
		fd.WriteOnly = true
	}
	if strings.Contains(lower, "beta") || strings.Contains(lower, "provider_versions") {
		fd.Beta = true
	}
	if strings.Contains(lower, "deprecated") {
		fd.Deprecated = true
	}
}

func stripStructuralPointerSentences(text string) string {
	if text == "" {
		return ""
	}
	if strings.HasSuffix(strings.TrimRight(text, " \t\n"), ":") {
		text = stripTrailingInlineBlockHeaderRe.ReplaceAllString(text, "")
	}
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "below") && !strings.Contains(lower, "above") {
		return text
	}
	sentinel := "\x00REMOVED_PTR\x00"

	replaceWithPunct := func(re *regexp.Regexp, in string) string {
		return re.ReplaceAllStringFunc(in, func(m string) string {
			sub := re.FindStringSubmatch(m)
			// If the pointer followed `;` or `,` (e.g. `A list of API objects; structure is [documented below](#nested_apis).`),
			// preserve the sentence-ending period if present.
			if len(sub) >= 3 && (sub[1] == ";" || sub[1] == ",") {
				if sub[2] == "." {
					return "."
				}
				return ""
			}
			return sentinel
		})
	}

	s := text
	s = replaceWithPunct(stripStructureLinkRe, s)
	s = stripSeeBelowNestedRe.ReplaceAllString(s, sentinel)
	s = stripMoreDetailsBelowRe.ReplaceAllString(s, sentinel)
	s = stripBareDocBelowLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, ".") {
			return "." + sentinel
		}
		return sentinel
	})
	s = replaceWithPunct(stripStructurePlainRe, s)
	s = stripConfigDetailedBelowRe.ReplaceAllString(s, sentinel)

	if !strings.Contains(s, sentinel) {
		return s
	}

	// Clean up sentinel per line:
	// - If an entire line consisted ONLY of the stripped pointer sentence (plus whitespace), drop that line!
	// - If the pointer sentence was inline alongside other sentences on the same line, collapse double spaces around it.
	rawLines := strings.Split(s, "\n")
	outLines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if !strings.Contains(line, sentinel) {
			outLines = append(outLines, line)
			continue
		}
		without := strings.ReplaceAll(line, sentinel, "")
		if strings.TrimSpace(without) == "" {
			// Entire line was the structural pointer sentence; omit line to avoid spurious blank line.
			continue
		}
		// Collapse double spaces introduced at splice sites while preserving leading indentation!
		lead := countLeadingSpaces(without)
		body := collapseDoubleSpaces(strings.TrimSpace(without))
		outLines = append(outLines, strings.Repeat(" ", lead)+body)
	}
	return strings.Join(outLines, "\n")
}

func collapseDoubleSpaces(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func expandLeadingTabs(line string) string {
	i := 0
	var b strings.Builder
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		if line[i] == '\t' {
			b.WriteString("    ")
		} else {
			b.WriteByte(' ')
		}
		i++
	}
	if i == 0 {
		return line
	}
	b.WriteString(line[i:])
	return b.String()
}

func countLeadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func countLeadingSpacesExpanded(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			n++
		} else if s[i] == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

func stripNSpaces(s string, n int) string {
	i := 0
	for i < len(s) && i < n && s[i] == ' ' {
		i++
	}
	return s[i:]
}

func trimBlankLines(lines []string) string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

func collapseExcessBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}
