package markdown

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ResourceDoc represents the parsed Registry Markdown documentation for a resource.
type ResourceDoc struct {
	ResourceName    string               // e.g. "google_compute_instance" (or "google_storage_bucket_iam" for *_iam.html.markdown)
	DocPath         string               // path passed in
	IsIAM           bool                 // true for *_iam.html.markdown*
	IsTemplate      bool                 // true for *.tmpl (e.g. tags_tag_binding_collection.html.markdown.tmpl)
	IsBetaOnly      bool                 // true if resource-level beta warning banner is present
	Subcategory     string               // from YAML frontmatter
	PageDescription string               // from YAML frontmatter description
	Overview        string               // resource overview prose before ## Example Usage / ## Argument Reference (stripped of H1 title & beta warning)
	Fields          map[string]*FieldDoc // keyed by canonical dot-separated schema path, e.g. "boot_disk", "boot_disk.initialize_params.size"
}

// FieldDoc represents the parsed documentation for a single schema field or nested block.
type FieldDoc struct {
	Name             string // leaf field name, e.g. "size"
	Path             string // canonical dot-separated path, e.g. "boot_disk.initialize_params.size"
	Section          string // "argument" or "attribute"
	RawDescription   string // raw text after "* `name` -"
	CleanDescription string // deterministically normalized Markdown prose ready for Go Schema.Description
	Required         bool
	Optional         bool
	Output           bool
	WriteOnly        bool
	Beta             bool
	Deprecated       bool
}

var (
	// Standalone Go template control directive line in .tmpl files, e.g. {{- if ne $.TargetVersionName "ga" }} or {{- end }}.
	tmplControlLineRe = regexp.MustCompile(`^[ \t]*\{\{-?[ \t]*(?:if|else|range|with|define|template|block|end)\b.*\}\}[ \t]*$`)

	// Resource-level beta warning banner in overview prose.
	betaBannerRe = regexp.MustCompile(`(?im)^[ \t]*(?:~>|!>|->|>)?[ \t]*\*\*Warning:?\*\*[ \t]*Th(?:is|ese)[ \t]+(?:resource|resources|data source|data sources)[ \t]+(?:is|are)[ \t]+in[ \t]+beta\b[^\n]*(?:\n[ \t]*See[ \t]+\[Provider Versions\][^\n]*)?[ \t]*$`)

	// Horizontal rule on its own line (- - - or ---).
	horizontalRuleRe = regexp.MustCompile(`^[ \t]*(?:- - -|-{3,})[ \t]*$`)

	// HTML anchor tag with non-empty name attribute, e.g. <a name="nested_boot_disk"></a>.
	anchorLineRe = regexp.MustCompile(`^[ \t]*<a[ \t]+name="([a-zA-Z0-9_-]+)"[^>]*>(?:</a>)?(.*)$`)

	// Unanchored nested block header start line, e.g.:
	//   The `condition` block supports:
	//   The `web_server_network_access_control` supports:
	//   The optional `settings.ip_configuration` subblock supports:
	//   The optional `settings.performance_capture_config` (Beta) subblock ... It contains:
	//   The optional `replica_configuration` block must have ... and supports:
	unanchoredBlockStartRe = regexp.MustCompile("^[ \\t]*The[ \\t]+(?:(?:optional|required|computed|read[- ]only)[,\\s]+)*`([a-zA-Z0-9_.\\[\\]-]+)`(?:[ \\t]+(?:or|and)[ \\t]+`([a-zA-Z0-9_.\\[\\]-]+)`)?(?:[ \\t]*\\([^)\\n]*\\))?[ \\t]+(?:(?:block|blocks|subblock|subblocks|sublist|sublists|list|object|structure)\\b|(?:supports|support|contains|holds):[ \\t]*$)")

	// Subject-clause cutoff so prose mentions after `block` (like `must have `master_instance_name` set`)
	// are not mistaken for block header names.
	blockKeywordCutoffRe = regexp.MustCompile(`\b(?:block|blocks|subblock|subblocks|sublist|sublists|list|object|structure|supports|support|contains|holds)\b`)

	// Backticked identifier extractor for block headers.
	backtickIdentRe = regexp.MustCompile("`([a-zA-Z0-9_.\\[\\]-]+)`")

	// Top-level bullet starting a documented field at column 0:
	//   * `name` - ...
	//   - `name` - ...
	//   * `name`: ...
	//   * `name` (Optional) - ...
	//   * The optional `psc_config.psc_auto_connections` subblock - (Optional) ...
	fieldBulletLineRe = regexp.MustCompile("^[*-][ \\t]+(?:The[ \\t]+(?:optional[ \\t]+)?)?`([a-zA-Z0-9_./*\\[\\]-]+)`(?:[ \\t]+subblock)?(.*)$")

	// Indented nested schema bullet inside a parent block bullet (e.g. dataproc_cluster & container_cluster):
	// Requires lowercase snake_case field name and explicit schema badge `(Required|Optional|Computed|Output|Beta|Deprecated|Input only|Read only|Write only)`
	// (or `Optional.` / `Required.`) to enter nested-schema-bullet mode without matching enum value bullets.
	indentedSchemaBulletEntryRe = regexp.MustCompile("^[ \\t]+[*-][ \\t]+`([a-z][a-z0-9_]*)`([ \\t]*(?:--?[ \\t]*|:[ \\t]*)?(?:\\((?:Required|Optional|Computed|Output|Beta|Deprecated|(?i:input[- ]only|read[- ]only|write[- ]only))\\b|(?:Optional|Required|Output)\\.).*)$")

	// Once already inside an indented nested-schema-bullet tree, sibling/child snake_case bullets with `-` or `:`
	// (e.g. `day: The day of the month` under `delay_until: (Optional) ...`) are also nested schema fields.
	indentedSchemaBulletContRe = regexp.MustCompile("^[ \\t]+[*-][ \\t]+`([a-z][a-z0-9_]*)`([ \\t]*(?:--?|:[ \\t]).*)$")

	// Section-level bullet group divider lines at column 0 that introduce a subset of bullets
	// rather than continuing the preceding bullet's description.
	bulletGroupDividerRe = regexp.MustCompile("^(?:(?:For[ \\t]+)?`google_[a-z0-9_]+_iam_(?:policy|binding|member|audit_config)`[^:\\n]*:|One of the following is required:|The following [a-zA-Z0-9_ ]+ are supported:|In addition, the `[a-zA-Z0-9_.]+` allows access to the following [^:\\n]+:)[ \\t]*$")

	// Resource-level delegation footer in compute_instance_from_template / compute_instance_from_machine_image.
	delegationFooterRe = regexp.MustCompile("^In addition to these,[ \\t]+(?:all|most\\*?)[ \\t]+arguments from `google_[a-z0-9_]+`")

	// Intra-page hash links in field descriptions, e.g. (#nested_boot_disk) or (#rotation_config).
	hashLinkRe = regexp.MustCompile(`\(#([a-zA-Z0-9_-]+)\)`)
)

// ParseFile reads and parses a Registry Markdown resource documentation file.
func ParseFile(path string) (*ResourceDoc, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read doc file %s: %w", path, err)
	}
	return Parse(path, src)
}

// ParseDir parses all Registry Markdown resource documentation files in docsRDir
// and returns a map keyed by canonical ResourceName.
func ParseDir(docsRDir string) (map[string]*ResourceDoc, error) {
	entries, err := os.ReadDir(docsRDir)
	if err != nil {
		return nil, fmt.Errorf("read docs dir %s: %w", docsRDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if isDocFilename(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	out := make(map[string]*ResourceDoc, len(names))
	for _, name := range names {
		fullPath := filepath.Join(docsRDir, name)
		doc, err := ParseFile(fullPath)
		if err != nil {
			return nil, err
		}
		if prev, exists := out[doc.ResourceName]; exists {
			return nil, fmt.Errorf("duplicate resource %q in %s and %s", doc.ResourceName, prev.DocPath, fullPath)
		}
		out[doc.ResourceName] = doc
	}
	return out, nil
}

func isDocFilename(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	n := strings.TrimSuffix(name, ".tmpl")
	return strings.HasSuffix(n, ".markdown") || strings.HasSuffix(n, ".md")
}

// Parse parses a Registry Markdown resource documentation buffer.
func Parse(filename string, src []byte) (*ResourceDoc, error) {
	resName, isIAM, isTmpl := deriveResourceIdentity(filename)
	if resName == "" {
		return nil, fmt.Errorf("cannot derive resource name from filename %q", filename)
	}

	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	if isTmpl {
		text = stripTemplateControlLines(text)
	}

	subcat, pageDesc, body := parseFrontmatter(text)
	overviewRaw, sections := splitOverviewAndSections(body)
	overview, isBetaOnly := cleanOverview(overviewRaw)

	doc := &ResourceDoc{
		ResourceName:    resName,
		DocPath:         filename,
		IsIAM:           isIAM,
		IsTemplate:      isTmpl,
		IsBetaOnly:      isBetaOnly,
		Subcategory:     subcat,
		PageDescription: pageDesc,
		Overview:        overview,
		Fields:          make(map[string]*FieldDoc),
	}

	// Collect all segments across argument and attribute sections so cross-section
	// anchor references (if any) can also resolve cleanly.
	var argSections [][]segment
	var attrSections [][]segment
	allAnchors := make(map[string][]*segment)

	for _, sec := range sections {
		switch sec.kind {
		case "argument":
			segs := splitSectionSegments(sec.lines, "argument")
			for i := range segs {
				if segs[i].anchor != "" {
					allAnchors[segs[i].anchor] = append(allAnchors[segs[i].anchor], &segs[i])
				}
			}
			argSections = append(argSections, segs)
		case "attribute":
			segs := splitSectionSegments(sec.lines, "attribute")
			for i := range segs {
				if segs[i].anchor != "" {
					allAnchors[segs[i].anchor] = append(allAnchors[segs[i].anchor], &segs[i])
				}
			}
			attrSections = append(attrSections, segs)
		}
	}

	for _, segs := range argSections {
		resolveSectionFields(doc, segs, "argument", allAnchors)
	}
	for _, segs := range attrSections {
		resolveSectionFields(doc, segs, "attribute", allAnchors)
	}

	return doc, nil
}

func deriveResourceIdentity(filename string) (resourceName string, isIAM bool, isTemplate bool) {
	base := filepath.Base(filename)
	if base == "." || base == "/" || base == "" {
		return "", false, false
	}
	if strings.HasSuffix(base, ".tmpl") {
		isTemplate = true
		base = strings.TrimSuffix(base, ".tmpl")
	}
	switch {
	case strings.HasSuffix(base, ".html.markdown"):
		base = strings.TrimSuffix(base, ".html.markdown")
	case strings.HasSuffix(base, ".html.md"):
		base = strings.TrimSuffix(base, ".html.md")
	case strings.HasSuffix(base, ".markdown"):
		base = strings.TrimSuffix(base, ".markdown")
	case strings.HasSuffix(base, ".md"):
		base = strings.TrimSuffix(base, ".md")
	}
	if base == "" {
		return "", false, isTemplate
	}
	isIAM = strings.HasSuffix(base, "_iam")
	if strings.HasPrefix(base, "google_") {
		resourceName = base
	} else {
		resourceName = "google_" + base
	}
	return resourceName, isIAM, isTemplate
}

func stripTemplateControlLines(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence && tmplControlLineRe.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func parseFrontmatter(text string) (subcategory, description, body string) {
	lines := strings.Split(text, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) || strings.TrimSpace(lines[start]) != "---" {
		return "", "", text
	}
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", "", text
	}

	fmLines := lines[start+1 : end]
	kv := parseSimpleYAMLMap(fmLines)
	subcategory = kv["subcategory"]
	description = kv["description"]
	body = strings.Join(lines[end+1:], "\n")
	return subcategory, description, body
}

func parseSimpleYAMLMap(lines []string) map[string]string {
	res := make(map[string]string)
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		// Top-level YAML key starts at column 0 (or minimal indent).
		colonIdx := strings.IndexByte(line, ':')
		if colonIdx <= 0 || line[0] == ' ' || line[0] == '\t' {
			i++
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		valPart := strings.TrimSpace(line[colonIdx+1:])
		i++

		isBlockScalar := valPart == "|" || valPart == "|-" || valPart == "|+" ||
			valPart == ">" || valPart == ">-" || valPart == ">+" || valPart == ""

		if isBlockScalar {
			var blockLines []string
			for i < len(lines) {
				next := lines[i]
				if strings.TrimSpace(next) == "" {
					blockLines = append(blockLines, "")
					i++
					continue
				}
				if next[0] == ' ' || next[0] == '\t' {
					blockLines = append(blockLines, next)
					i++
					continue
				}
				break
			}
			res[key] = dedentYAMLBlock(blockLines)
		} else {
			res[key] = unquoteYAMLScalar(valPart)
		}
	}
	return res
}

func unquoteYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}

func dedentYAMLBlock(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := countLeadingSpacesExpanded(l)
		if minIndent == -1 || ind < minIndent {
			minIndent = ind
		}
	}
	if minIndent < 0 {
		return ""
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			out[i] = ""
			continue
		}
		expanded := expandLeadingTabs(l)
		if len(expanded) >= minIndent {
			expanded = expanded[minIndent:]
		}
		out[i] = strings.TrimRight(expanded, " \t")
	}
	return trimBlankLines(out)
}

type rawSection struct {
	header string
	kind   string // "argument", "attribute", or "other"
	lines  []string
}

func splitOverviewAndSections(body string) (string, []rawSection) {
	lines := strings.Split(body, "\n")
	var overviewLines []string
	var sections []rawSection
	curIdx := -1
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			hdr := strings.TrimSpace(line[3:])
			sections = append(sections, rawSection{
				header: hdr,
				kind:   classifySectionHeader(hdr),
			})
			curIdx = len(sections) - 1
			continue
		}
		if curIdx == -1 {
			overviewLines = append(overviewLines, line)
		} else {
			sections[curIdx].lines = append(sections[curIdx].lines, line)
		}
	}
	return strings.Join(overviewLines, "\n"), sections
}

func classifySectionHeader(hdr string) string {
	lower := strings.ToLower(strings.TrimSpace(hdr))
	switch {
	case strings.HasPrefix(lower, "argument reference"),
		strings.HasPrefix(lower, "arguments reference"):
		return "argument"
	case strings.HasPrefix(lower, "attributes reference"),
		strings.HasPrefix(lower, "attribute reference"),
		strings.HasPrefix(lower, "ephemeral attributes reference"),
		strings.HasPrefix(lower, "ephemeral attribute reference"):
		return "attribute"
	default:
		return "other"
	}
}

func cleanOverview(raw string) (overview string, isBetaOnly bool) {
	lines := strings.Split(raw, "\n")
	var withoutH1 []string
	strippedH1 := false
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence && !strippedH1 && strings.HasPrefix(trimmed, "# ") {
			strippedH1 = true
			continue
		}
		withoutH1 = append(withoutH1, strings.TrimRight(line, " \t"))
	}
	joined := strings.Join(withoutH1, "\n")
	if betaBannerRe.MatchString(joined) {
		isBetaOnly = true
		joined = betaBannerRe.ReplaceAllString(joined, "")
	}
	cleanedLines := strings.Split(joined, "\n")
	return collapseExcessBlankLines(trimBlankLines(cleanedLines)), isBetaOnly
}

type segmentKind int

const (
	segRoot segmentKind = iota
	segAnchored
	segUnanchored
)

type segment struct {
	pos         int
	section     string // "argument" or "attribute"
	kind        segmentKind
	anchor      string   // e.g. "nested_boot_disk"
	headerNames []string // backticked names in block header, e.g. ["boot_disk"] or ["allow", "deny"]
	isSubblock  bool     // true if header explicitly says "subblock" or "sublist"
	bullets     []rawBullet
	used        bool
}

type rawBullet struct {
	rawName string
	rawDesc string
	links   []string // anchor targets from (#...)
}

func splitSectionSegments(lines []string, sectionKind string) []segment {
	type rawChunk struct {
		kind       segmentKind
		anchor     string
		headerLine string
		bodyLines  []string
	}

	chunks := []rawChunk{{kind: segRoot}}
	inFence := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			chunks[len(chunks)-1].bodyLines = append(chunks[len(chunks)-1].bodyLines, line)
			continue
		}
		if inFence {
			chunks[len(chunks)-1].bodyLines = append(chunks[len(chunks)-1].bodyLines, line)
			continue
		}

		// Horizontal rule resets to a root segment for any top-level bullets following it.
		if horizontalRuleRe.MatchString(line) {
			chunks = append(chunks, rawChunk{kind: segRoot})
			continue
		}

		// Ignore ### / #### subheading lines inside Argument/Attribute Reference (e.g. endpoints_service).
		if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
			continue
		}

		// Ignore bare broken <a name> or <a id="timeouts"></a> lines.
		if trimmed == "<a name>" || strings.HasPrefix(trimmed, `<a id="`) {
			continue
		}

		// Check <a name="..."> anchored block start.
		if m := anchorLineRe.FindStringSubmatch(line); m != nil {
			anchorName := m[1]
			rest := strings.TrimSpace(m[2])
			hdr := rest
			// Handle wrapped header line like:
			// <a name="nested_data_retention_config_gen_3"></a>The `data_retention_config`
			// block supports:
			if rest != "" && !strings.HasSuffix(rest, ":") && !strings.HasSuffix(rest, "-") && i+1 < len(lines) {
				nextTrim := strings.TrimSpace(lines[i+1])
				if nextTrim != "" && !strings.HasPrefix(nextTrim, "*") && !strings.HasPrefix(nextTrim, "-") && strings.HasSuffix(nextTrim, ":") {
					hdr = rest + " " + nextTrim
					i++
				}
			}
			chunks = append(chunks, rawChunk{
				kind:       segAnchored,
				anchor:     anchorName,
				headerLine: hdr,
			})
			continue
		}

		// Check unanchored nested block header line (single-line or up to 3-line intro paragraph ending in `:`).
		if unanchoredBlockStartRe.MatchString(line) {
			if strings.HasSuffix(trimmed, ":") {
				chunks = append(chunks, rawChunk{
					kind:       segUnanchored,
					headerLine: trimmed,
				})
				continue
			}
			endLine := -1
			combined := trimmed
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				jt := strings.TrimSpace(lines[j])
				if jt == "" || strings.HasPrefix(jt, "*") || strings.HasPrefix(jt, "-") || strings.HasPrefix(jt, "#") || strings.HasPrefix(jt, "<a ") {
					break
				}
				combined += " " + jt
				if strings.HasSuffix(jt, ":") {
					endLine = j
					break
				}
			}
			if endLine != -1 {
				chunks = append(chunks, rawChunk{
					kind:       segUnanchored,
					headerLine: combined,
				})
				i = endLine
				continue
			}
		}

		chunks[len(chunks)-1].bodyLines = append(chunks[len(chunks)-1].bodyLines, line)
	}

	segs := make([]segment, 0, len(chunks))
	for _, ch := range chunks {
		bullets := extractBullets(ch.bodyLines)
		if ch.kind == segRoot && len(bullets) == 0 {
			continue
		}
		hdrSubject := ch.headerLine
		if loc := blockKeywordCutoffRe.FindStringIndex(hdrSubject); loc != nil {
			if strings.Contains(hdrSubject[:loc[0]], "`") {
				hdrSubject = hdrSubject[:loc[0]]
			}
		}
		var hdrNames []string
		for _, bm := range backtickIdentRe.FindAllStringSubmatch(hdrSubject, -1) {
			cleaned := cleanFieldToken(bm[1])
			if cleaned != "" {
				hdrNames = append(hdrNames, cleaned)
			}
		}
		lowerHdr := strings.ToLower(ch.headerLine)
		isSub := strings.Contains(lowerHdr, "subblock") || strings.Contains(lowerHdr, "sublist")
		segs = append(segs, segment{
			pos:         len(segs),
			section:     sectionKind,
			kind:        ch.kind,
			anchor:      ch.anchor,
			headerNames: hdrNames,
			isSubblock:  isSub,
			bullets:     bullets,
		})
	}
	return segs
}

func extractBullets(lines []string) []rawBullet {
	type stackEntry struct {
		indent int
		path   string
	}
	var bullets []rawBullet
	var stack []stackEntry
	curIdx := -1
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			if curIdx >= 0 {
				bullets[curIdx].rawDesc += "\n" + line
			}
			continue
		}
		if !inFence {
			if bulletGroupDividerRe.MatchString(line) {
				curIdx = -1
				stack = nil
				continue
			}
			if delegationFooterRe.MatchString(line) {
				curIdx = -1
				stack = nil
				continue
			}
			if m := fieldBulletLineRe.FindStringSubmatch(line); m != nil {
				name := m[1]
				rest := m[2]
				bullets = append(bullets, rawBullet{
					rawName: name,
					rawDesc: stripLeadingBulletSeparator(rest),
				})
				curIdx = len(bullets) - 1
				stack = []stackEntry{{indent: 0, path: name}}
				continue
			}
			if len(stack) > 0 {
				popInd := countLeadingSpacesExpanded(line)
				pushInd := bulletIndentColumns(line)
				if popInd > 0 {
					var subMatch []string
					if subMatch = indentedSchemaBulletEntryRe.FindStringSubmatch(line); subMatch == nil && len(stack) > 1 {
						subMatch = indentedSchemaBulletContRe.FindStringSubmatch(line)
					}
					if subMatch != nil {
						childName := subMatch[1]
						rest := subMatch[2]
						for len(stack) > 1 && stack[len(stack)-1].indent >= popInd {
							stack = stack[:len(stack)-1]
						}
						parentPath := stack[len(stack)-1].path
						relPath := parentPath + "." + childName
						bullets = append(bullets, rawBullet{
							rawName: relPath,
							rawDesc: stripLeadingBulletSeparator(rest),
						})
						curIdx = len(bullets) - 1
						stack = append(stack, stackEntry{indent: pushInd, path: relPath})
						continue
					}
				}
			}
		}
		if curIdx >= 0 {
			bullets[curIdx].rawDesc += "\n" + line
		}
	}

	for i := range bullets {
		bullets[i].links = extractHashLinks(bullets[i].rawDesc)
	}
	return bullets
}

func stripLeadingBulletSeparator(s string) string {
	trimmed := strings.TrimLeft(s, " \t")
	switch {
	case strings.HasPrefix(trimmed, "--"):
		return strings.TrimLeft(trimmed[2:], " \t")
	case strings.HasPrefix(trimmed, "-"):
		return strings.TrimLeft(trimmed[1:], " \t")
	case strings.HasPrefix(trimmed, ":"):
		return strings.TrimLeft(trimmed[1:], " \t")
	default:
		return trimmed
	}
}

func extractHashLinks(desc string) []string {
	matches := hashLinkRe.FindAllStringSubmatch(desc, -1)
	if len(matches) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		target := m[1]
		if !seen[target] {
			seen[target] = true
			out = append(out, target)
		}
	}
	return out
}

func bulletIndentColumns(s string) int {
	col := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			col++
		case '\t':
			col = (col/8 + 1) * 8
		default:
			return col
		}
	}
	return col
}

type discoveredField struct {
	path         string
	leaf         string
	segPos       int
	hasStructPtr bool
	expanded     bool
}

func resolveSectionFields(doc *ResourceDoc, segs []segment, sectionKind string, allAnchors map[string][]*segment) {
	// Build section-local anchor map first so same-section anchors take precedence.
	localAnchors := make(map[string][]*segment)
	for i := range segs {
		if segs[i].anchor != "" {
			localAnchors[segs[i].anchor] = append(localAnchors[segs[i].anchor], &segs[i])
		}
	}

	var discovered []discoveredField
	lastBlockPrefix := ""

	var expandSegment func(seg *segment, prefix string, activeAnchors map[string]bool)
	expandSegment = func(seg *segment, prefix string, activeAnchors map[string]bool) {
		seg.used = true
		if prefix != "" {
			lastBlockPrefix = prefix
		}
		for _, b := range seg.bullets {
			normRel := normalizeRelativeFieldPath(b.rawName)
			if normRel == "" {
				continue
			}

			// Handle IAM member/members expansion at top-level or nested if applicable.
			if doc.IsIAM && normRel == "member/members" && prefix == "" {
				fdMember := buildFieldDoc("member", "member", sectionKind, b.rawDesc)
				fdMembers := buildFieldDoc("members", "members", sectionKind, b.rawDesc)
				upsertFieldDoc(doc, fdMember)
				upsertFieldDoc(doc, fdMembers)
				continue
			}

			fullPath := joinFieldPath(prefix, normRel)
			leaf := leafFieldName(fullPath)
			fd := buildFieldDoc(leaf, fullPath, sectionKind, b.rawDesc)
			upsertFieldDoc(doc, fd)

			discIdx := len(discovered)
			discovered = append(discovered, discoveredField{
				path:         fullPath,
				leaf:         leaf,
				segPos:       seg.pos,
				hasStructPtr: len(b.links) == 0 && hasUnlinkedStructuralPointer(b.rawDesc),
			})

			// Resolve explicit (#anchor) nested block pointers on this bullet.
			for _, link := range b.links {
				if activeAnchors[link] {
					continue
				}
				targetSeg := selectAnchorSegment(link, seg.pos, leaf, b.rawDesc, localAnchors, allAnchors)
				if targetSeg == nil {
					continue
				}
				activeAnchors[link] = true
				discovered[discIdx].expanded = true
				expandSegment(targetSeg, fullPath, activeAnchors)
				delete(activeAnchors, link)
			}
		}
	}

	// Pass 1: Process root segments and follow all (#anchor) links in DFS order,
	// while also resolving unanchored (or unlinked anchored) nested block segments
	// as they appear in document order!
	for i := range segs {
		seg := &segs[i]
		if seg.kind == segRoot {
			expandSegment(seg, "", make(map[string]bool))
			continue
		}
		if seg.used {
			continue
		}
		// If this anchored segment is referenced by a LATER segment's (#anchor) link
		// (e.g. a later root chunk after `- - -`, or a block documented above its parent),
		// defer it until that (#anchor) link visits it—if still unused at the end, Pass 2 handles it.
		if seg.kind == segAnchored && isReferencedLater(seg.anchor, seg.pos, segs) {
			continue
		}
		prefixes := inferUnlinkedBlockPrefixes(seg, segs, discovered, lastBlockPrefix)
		for _, pfx := range prefixes {
			markExpanded(discovered, pfx)
			active := make(map[string]bool)
			if seg.anchor != "" {
				active[seg.anchor] = true
			}
			expandSegment(seg, pfx, active)
		}
	}

	// Pass 2: Any anchored segment that was deferred because its anchor name appeared later,
	// or was never visited via (#anchor), gets resolved now.
	for i := range segs {
		seg := &segs[i]
		if seg.used || seg.kind == segRoot {
			continue
		}
		prefixes := inferUnlinkedBlockPrefixes(seg, segs, discovered, lastBlockPrefix)
		for _, pfx := range prefixes {
			markExpanded(discovered, pfx)
			active := make(map[string]bool)
			if seg.anchor != "" {
				active[seg.anchor] = true
			}
			expandSegment(seg, pfx, active)
		}
	}
}

func selectAnchorSegment(anchor string, parentPos int, fieldLeaf string, rawDesc string, localAnchors, allAnchors map[string][]*segment) *segment {
	candidates := localAnchors[anchor]
	if len(candidates) == 0 {
		candidates = allAnchors[anchor]
	}
	if len(candidates) == 0 {
		return nil
	}

	// Choose the first candidate strictly after parentPos (matching DFS document order
	// for repeated anchor names like nested_source/nested_file in os_config_os_policy_assignment),
	// falling back to the first candidate overall (for "documented above" references).
	var chosen *segment
	for _, c := range candidates {
		if c.pos > parentPos {
			chosen = c
			break
		}
	}
	if chosen == nil {
		chosen = candidates[0]
	}

	// Guard against non-structural cross-references to another field's anchor
	// (e.g., bigquery_table's `schema` field saying:
	// "If you use `external_data_configuration` [documented below](#nested_external_data_configuration) and do not set...").
	if !isStructuralNestedTarget(anchor, fieldLeaf, rawDesc, chosen) {
		return nil
	}
	return chosen
}

func isStructuralNestedTarget(anchor, fieldLeaf, rawDesc string, target *segment) bool {
	// 1. Anchor explicitly matches field leaf (e.g. nested_foo, foo, nested_parent_foo).
	lowerAnchor := strings.ToLower(anchor)
	lowerLeaf := strings.ToLower(fieldLeaf)
	if lowerAnchor == lowerLeaf ||
		lowerAnchor == "nested_"+lowerLeaf ||
		strings.HasSuffix(lowerAnchor, "_"+lowerLeaf) {
		return true
	}

	// 2. Target segment's block header explicitly names this field leaf
	// (e.g. `The `allow` or `deny` blocks support:` or `The `filter` block supports:`).
	for _, hn := range target.headerNames {
		if strings.EqualFold(leafFieldName(hn), lowerLeaf) {
			return true
		}
	}

	// 3. The link appears inside an explicit structural pointer sentence
	// (handles typos in block headers like `The `loggin_config` block supports:` or
	// abbreviated anchors like `nested_managed_ml_diagnostics_config` for `managed_machine_learning_diagnostics_config`).
	return hasStructuralPointerToAnchor(rawDesc, anchor)
}

func isReferencedLater(anchor string, pos int, segs []segment) bool {
	if anchor == "" {
		return false
	}
	for i := pos + 1; i < len(segs); i++ {
		for _, b := range segs[i].bullets {
			for _, l := range b.links {
				if l == anchor {
					return true
				}
			}
		}
	}
	return false
}

func inferUnlinkedBlockPrefixes(seg *segment, segs []segment, discovered []discoveredField, lastBlockPrefix string) []string {
	if len(seg.headerNames) == 0 {
		return nil
	}
	var prefixes []string
	seen := make(map[string]bool)
	for _, hn := range seg.headerNames {
		for _, p := range matchHeaderNameToPrefixes(hn, seg, segs, discovered, lastBlockPrefix) {
			if p != "" && !seen[p] {
				seen[p] = true
				prefixes = append(prefixes, p)
			}
		}
	}
	return prefixes
}

func hasLaterSegmentWithHeader(hn string, pos int, segs []segment) bool {
	for i := pos + 1; i < len(segs); i++ {
		for _, other := range segs[i].headerNames {
			if other == hn {
				return true
			}
		}
	}
	return false
}

func matchHeaderNameToPrefixes(hn string, seg *segment, segs []segment, discovered []discoveredField, lastBlockPrefix string) []string {
	// Case 1: Header name already contains dots (e.g. "settings.ip_configuration" or
	// "master_authorized_networks_config.cidr_blocks" or "settings.ip_configuration.authorized_networks").
	if strings.Contains(hn, ".") {
		for i := len(discovered) - 1; i >= 0; i-- {
			if discovered[i].path == hn || strings.HasSuffix(discovered[i].path, "."+hn) {
				return []string{discovered[i].path}
			}
		}
		// Check if prefix before last dot was discovered (e.g. "settings.ip_configuration" discovered,
		// and header says "settings.ip_configuration.authorized_networks" even if authorized_networks bullet was omitted).
		if dot := strings.LastIndexByte(hn, '.'); dot > 0 {
			parentPart := hn[:dot]
			leafPart := hn[dot+1:]
			for i := len(discovered) - 1; i >= 0; i-- {
				if discovered[i].path == parentPart || strings.HasSuffix(discovered[i].path, "."+parentPart) {
					return []string{discovered[i].path + "." + leafPart}
				}
			}
		}
		return []string{hn}
	}

	// Case 2: Single identifier header name (e.g. "condition", "postdeploy", "verify_config", "disruption_budget").
	var unexpanded []discoveredField
	for _, d := range discovered {
		if d.leaf == hn && !d.expanded {
			unexpanded = append(unexpanded, d)
		}
	}
	if len(unexpanded) > 0 {
		var ptrMatches []discoveredField
		for _, d := range unexpanded {
			if d.hasStructPtr {
				ptrMatches = append(ptrMatches, d)
			}
		}
		if len(ptrMatches) > 0 {
			unexpanded = ptrMatches
		}

		// When an unanchored block header (e.g. `The `verify_config` block supports:` in clouddeploy_delivery_pipeline)
		// is shared across multiple unexpanded sibling fields and no later segment re-declares `hn`,
		// expand all unexpanded sibling paths!
		if len(unexpanded) > 1 && seg.kind == segUnanchored && !hasLaterSegmentWithHeader(hn, seg.pos, segs) {
			out := make([]string, 0, len(unexpanded))
			for _, d := range unexpanded {
				out = append(out, d.path)
			}
			return out
		}
		return []string{unexpanded[len(unexpanded)-1].path}
	}

	// Fallback to any discovered field with matching leaf name (preferring explicit structural pointer).
	for i := len(discovered) - 1; i >= 0; i-- {
		if discovered[i].leaf == hn && discovered[i].hasStructPtr {
			return []string{discovered[i].path}
		}
	}
	for i := len(discovered) - 1; i >= 0; i-- {
		if discovered[i].leaf == hn {
			return []string{discovered[i].path}
		}
	}
	// Fallback for subblocks whose parent omitted a bullet for the subblock itself
	// (e.g. sql_database_instance `The optional `final_backup_config` subblock supports:` right under `settings`).
	if seg.isSubblock && lastBlockPrefix != "" {
		topParent := strings.SplitN(lastBlockPrefix, ".", 2)[0]
		return []string{topParent + "." + hn}
	}
	return []string{hn}
}

func markExpanded(discovered []discoveredField, path string) {
	for i := range discovered {
		if discovered[i].path == path {
			discovered[i].expanded = true
		}
	}
}

func upsertFieldDoc(doc *ResourceDoc, cand *FieldDoc) {
	existing, exists := doc.Fields[cand.Path]
	if !exists {
		doc.Fields[cand.Path] = cand
		return
	}

	// Same section (e.g. composer_environment Gen 3 -> Gen 2 -> Gen 1, or repeated shared blocks):
	// keep the first non-empty CleanDescription for determinism.
	if existing.Section == cand.Section {
		if existing.CleanDescription == "" && cand.CleanDescription != "" {
			doc.Fields[cand.Path] = cand
		}
		return
	}

	// Cross-section collision (Argument Reference vs Attributes Reference):
	// Prefer Argument Reference entry unless its CleanDescription is empty.
	if existing.Section == "argument" && cand.Section == "attribute" {
		if existing.CleanDescription == "" && cand.CleanDescription != "" {
			doc.Fields[cand.Path] = cand
		}
		return
	}
	if existing.Section == "attribute" && cand.Section == "argument" {
		if cand.CleanDescription != "" || existing.CleanDescription == "" {
			doc.Fields[cand.Path] = cand
		}
	}
}

func cleanFieldToken(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "[]", "")
	s = normalizeIndexedSegments(s)
	s = strings.Trim(s, ".")
	return s
}

func normalizeRelativeFieldPath(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "member/members" {
		return s
	}
	return cleanFieldToken(s)
}

func normalizeIndexedSegments(path string) string {
	parts := strings.Split(path, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSuffix(p, "[]")
		if p == "" || p == "0" || p == "*" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ".")
}

func joinFieldPath(prefix, rel string) string {
	if prefix == "" {
		return rel
	}
	if rel == "" {
		return prefix
	}
	if strings.HasPrefix(rel, prefix+".") {
		return rel
	}
	// Handle bullets inside block `a.b.c` that repeat `c.leaf` (e.g. `psc_config.psc_auto_connections` inside `settings.ip_configuration.psc_config`).
	if dot := strings.IndexByte(rel, '.'); dot > 0 {
		firstSeg := rel[:dot]
		rest := rel[dot+1:]
		if prefix == firstSeg || strings.HasSuffix(prefix, "."+firstSeg) {
			return prefix + "." + rest
		}
	}
	return prefix + "." + rel
}

func leafFieldName(path string) string {
	if idx := strings.LastIndexByte(path, '.'); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
