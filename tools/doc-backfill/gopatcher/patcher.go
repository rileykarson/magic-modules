package gopatcher

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FormatGoStringLiteral formats s as a deterministic, lossless Go string literal.
// If s contains no backticks (`), no carriage returns (\r), valid UTF-8, and no control
// characters other than \n and \t, a raw string literal (`...`) is emitted; otherwise
// strconv.Quote(s) ("...") is emitted so backticks and control characters survive verbatim.
func FormatGoStringLiteral(s string) string {
	if canUseRawStringLiteral(s) {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

func canUseRawStringLiteral(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r == '`' || r == '\r' {
			return false
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func formatLiteralForFile(s string, isTmpl bool) string {
	lit := FormatGoStringLiteral(s)
	if isTmpl {
		return escapeLiteralForGoTemplate(lit)
	}
	return lit
}

type spanEdit struct {
	start       int
	end         int
	replacement []byte
}

type pendingTargetPatch struct {
	targetKey string
	filePath  string
	newDesc   string
	meta      compositePatchMeta
	hasDesc   bool
}

// ApplyPatches applies non-overlapping surgical byte-span edits to .go and .go.tmpl files.
// Idempotent patches (where CurrentDescription already equals NewDescription) and non-literal
// targets produce no edits. Pure .go files with applied edits are formatted with gofmt
// (go/format.Source) and verified to parse cleanly.
func ApplyPatches(patches []PatchRequest, resPatches []ResourceDescPatch) ([]FileDiff, error) {
	seenTargets := make(map[string]string)
	var validPatches []pendingTargetPatch

	for _, req := range patches {
		ft := req.Target
		if ft == nil {
			continue
		}
		if !ft.IsLiteralOrMissing {
			continue
		}
		if ft.HasDescription && ft.CurrentDescription == req.NewDescription {
			// Idempotency: already matches exact description value.
			continue
		}
		targetKey := ft.TargetID
		if targetKey == "" {
			targetKey = fmt.Sprintf("%s:%d", ft.FilePath, ft.patchMeta.compStart)
		}
		if prevDesc, exists := seenTargets[targetKey]; exists {
			if prevDesc != req.NewDescription {
				return nil, fmt.Errorf("conflicting patch descriptions for shared target %s", targetKey)
			}
			continue
		}
		seenTargets[targetKey] = req.NewDescription
		validPatches = append(validPatches, pendingTargetPatch{
			targetKey: targetKey,
			filePath:  ft.FilePath,
			newDesc:   req.NewDescription,
			meta:      ft.patchMeta,
			hasDesc:   ft.HasDescription,
		})
	}

	for _, rp := range resPatches {
		rt := rp.Resource
		if rt == nil || !rt.CanPatchResDesc {
			continue
		}
		if rt.HasResDescription && rt.CurrentResDesc == rp.NewDescription {
			continue
		}
		filePath := rt.resPatchMeta.filePath
		if filePath == "" {
			filePath = rt.PrimaryFile
		}
		targetKey := fmt.Sprintf("res:%s:%d", filePath, rt.resPatchMeta.compStart)
		if prevDesc, exists := seenTargets[targetKey]; exists {
			if prevDesc != rp.NewDescription {
				return nil, fmt.Errorf("conflicting resource description patches for %s", rt.ResourceName)
			}
			continue
		}
		seenTargets[targetKey] = rp.NewDescription
		validPatches = append(validPatches, pendingTargetPatch{
			targetKey: targetKey,
			filePath:  filePath,
			newDesc:   rp.NewDescription,
			meta:      rt.resPatchMeta,
			hasDesc:   rt.HasResDescription,
		})
	}

	if len(validPatches) == 0 {
		return nil, nil
	}

	patchesByFile := make(map[string][]pendingTargetPatch)
	for _, p := range validPatches {
		patchesByFile[p.filePath] = append(patchesByFile[p.filePath], p)
	}

	filePaths := make([]string, 0, len(patchesByFile))
	for fp := range patchesByFile {
		filePaths = append(filePaths, fp)
	}
	sort.Strings(filePaths)

	var diffs []FileDiff
	for _, fp := range filePaths {
		origBytes, err := os.ReadFile(fp)
		if err != nil {
			return nil, fmt.Errorf("reading %s for patching: %w", fp, err)
		}
		isTmpl := strings.HasSuffix(fp, ".go.tmpl")
		filePatches := patchesByFile[fp]

		var edits []spanEdit
		appliedCount := 0
		for _, p := range filePatches {
			lit := formatLiteralForFile(p.newDesc, isTmpl)
			if p.hasDesc {
				if len(p.meta.descSpans) == 0 {
					return nil, fmt.Errorf("missing description byte span metadata for %s", p.targetKey)
				}
				for _, sp := range p.meta.descSpans {
					if sp.start < 0 || sp.end > len(origBytes) || sp.start > sp.end {
						return nil, fmt.Errorf("out-of-bounds byte span [%d,%d) in %s (len=%d)", sp.start, sp.end, fp, len(origBytes))
					}
					edits = append(edits, spanEdit{
						start:       sp.start,
						end:         sp.end,
						replacement: []byte(lit),
					})
				}
				appliedCount++
			} else {
				insOff := p.meta.insertOffset
				if insOff < 0 || insOff > len(origBytes) {
					return nil, fmt.Errorf("out-of-bounds insert offset %d in %s (len=%d)", insOff, fp, len(origBytes))
				}
				repl := p.meta.insertPrefix + lit + p.meta.insertSuffix
				edits = append(edits, spanEdit{
					start:       insOff,
					end:         insOff,
					replacement: []byte(repl),
				})
				appliedCount++
			}
		}

		if appliedCount == 0 || len(edits) == 0 {
			continue
		}

		// Sort edits in descending start order so applying them never shifts earlier offsets.
		sort.Slice(edits, func(i, j int) bool {
			if edits[i].start != edits[j].start {
				return edits[i].start > edits[j].start
			}
			return edits[i].end > edits[j].end
		})

		// Verify strictly non-overlapping spans (edits[i] is at a higher offset than edits[i+1]).
		for i := 0; i+1 < len(edits); i++ {
			higher := edits[i]
			lower := edits[i+1]
			if lower.end > higher.start || (lower.start == higher.start && lower.end == higher.end) {
				return nil, fmt.Errorf("overlapping byte edits in %s: [%d,%d) vs [%d,%d)", fp, lower.start, lower.end, higher.start, higher.end)
			}
		}

		updated := make([]byte, len(origBytes))
		copy(updated, origBytes)
		for _, ed := range edits {
			var buf bytes.Buffer
			buf.Grow(len(updated) - (ed.end - ed.start) + len(ed.replacement))
			buf.Write(updated[:ed.start])
			buf.Write(ed.replacement)
			buf.Write(updated[ed.end:])
			updated = buf.Bytes()
		}

		if !isTmpl {
			formatted, err := format.Source(updated)
			if err != nil {
				return nil, fmt.Errorf("gofmt failed after patching %s: %w", fp, err)
			}
			updated = formatted
		}

		if bytes.Equal(origBytes, updated) {
			continue
		}

		diffs = append(diffs, FileDiff{
			FilePath:      fp,
			OriginalBytes: origBytes,
			UpdatedBytes:  updated,
			AppliedCount:  appliedCount,
		})
	}

	return diffs, nil
}
