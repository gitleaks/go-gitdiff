package gitdiff

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ParseCombinedTextFragments parses text fragments with 2 or more parents until the
// next file header or the end of the stream and attaches them to the given file. It
// returns the number of fragments that were added.
func (p *parser) ParseCombinedTextFragments(f *File) (n int, err error) {
	for {
		frags, err := p.ParseCombinedTextFragmentHeader()
		if err != nil {
			return n, err
		}
		if len(frags) <= 0 {
			return n, nil
		}

		for _, frag := range frags {
			if f.IsNew && frag.OldLines > 0 {
				return n, p.Errorf(-1, "new file depends on old contents")
			}
			if f.IsDelete && frag.NewLines > 0 {
				return n, p.Errorf(-1, "deleted file still has contents")
			}
		}

		if err := p.ParseCombinedTextChunk(frags); err != nil {
			return n, err
		}

		f.TextFragments = append(f.TextFragments, frags...)
		n += len(frags)
	}
}

func (p *parser) ParseCombinedTextFragmentHeader() ([]*TextFragment, error) {
	// There are (number of parents + 1) @ characters in the chunk header for combined diff format.
	// This implementation is generic enough to use for both the standard '@@ ' text diff and for
	// the combined diff.  However, for stability and performance reasons, they are split into
	// different implementations.
	const (
		parentMark        = '@'
		minStartMark      = "@@@"
		trailingStartMark = "@ -"
	)
	line := p.Line(0)

	if !strings.HasPrefix(line, minStartMark) {
		return nil, nil
	}

	// Find wrapping markers around the range, and, in doing so, count the number of parent files.
	startEnd := strings.Index(line, trailingStartMark)
	if startEnd < 0 {
		return nil, nil
	}
	parentCount := 0
	endMark := " @"
	for ; parentCount < startEnd; parentCount++ {
		// check for valid combined form marker.
		if line[parentCount] != parentMark {
			return nil, nil
		}
		endMark += line[parentCount : parentCount+1]
	}

	// Split up the line into sections.
	// Keep the leading '-' on the first range.
	startPos := startEnd + len(trailingStartMark) - 1
	parts := strings.SplitAfterN(p.Line(0), endMark, 2)
	if len(parts) < 2 {
		return nil, p.Errorf(0, "invalid fragment header")
	}
	comment := strings.TrimSpace(parts[1])

	// Collect the file ranges.
	header := parts[0][startPos : len(parts[0])-len(endMark)]
	ranges := strings.Split(header, " ")
	if len(ranges) != parentCount+1 {
		return nil, p.Errorf(0, "invalid fragment header")
	}
	frags, err := parseCombinedHeaderRanges(comment, ranges)
	if err != nil {
		return nil, p.Errorf(0, "invalid fragment header: %v", err)
	}

	if err := p.Next(); err != nil && err != io.EOF {
		return nil, err
	}
	return frags, nil
}

func parseCombinedHeaderRanges(comment string, ranges []string) ([]*TextFragment, error) {
	parentCount := len(ranges) - 1

	// This needs to cope with a strange old-line range of '-1' that some versions
	// of Git produce.  When this happens, the old and new line counts must increment by 1.
	var negativeCount int64 = 0

	// Parse the merged range.
	var err error
	newPosition, newLines, err := parseCombinedRange(ranges[parentCount][1:])
	if err != nil {
		return nil, err
	}
	if newLines < 0 {
		negativeCount -= newLines
		// The newLines count remains negative, so it adjusts to zero at the end.
	}

	// Parse the parent file ranges.
	frags := make([]*TextFragment, parentCount)
	for i := 0; i < parentCount; i++ {
		f := &TextFragment{
			Comment:     comment,
			NewPosition: newPosition,
			NewLines:    newLines,
		}
		if f.OldPosition, f.OldLines, err = parseCombinedRange(ranges[i][1:]); err != nil {
			return nil, err
		}
		if f.OldLines < 0 {
			negativeCount -= f.OldLines
			// The OldLines count remains negative, so the final adjustment makes it zero.
		}
		frags[i] = f
	}

	// Adjust each fragment count based on the negative count.
	for _, f := range frags {
		f.OldLines += negativeCount
		f.NewLines += negativeCount
	}

	return frags, nil
}

func parseCombinedRange(s string) (start int64, end int64, err error) {
	parts := strings.SplitN(s, ",", 2)

	if start, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		nerr := err.(*strconv.NumError)
		return 0, 0, fmt.Errorf("bad start of range: %s: %v", parts[0], nerr.Err)
	}

	if len(parts) > 1 {
		if parts[1] == "18446744073709551615" {
			// There are some versions of "git diff --cc" that return a uint64 version of -1,
			// which is this number.  That seems to mean that, for this specific file,
			// there wasn't any change.
			// This is the only version of this large number that's been seen, though it's possible that
			// the git diff --cc format can return other negative numbers.  In those cases, more complex
			// logic would be needed to convert the uint64 to signed int64.
			end = -1
		} else if end, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			nerr := err.(*strconv.NumError)
			return 0, 0, fmt.Errorf("bad end of range: %s: %v", parts[1], nerr.Err)
		}
	} else {
		end = 1
	}

	return
}

func (p *parser) ParseCombinedTextChunk(frags []*TextFragment) error {
	if p.Line(0) == "" {
		return p.Errorf(0, "no content following fragment header")
	}
	parentCount := len(frags)

	// Track whether any line included an alteration.
	noLineChanges := true

	// Only count leading and trailing context when it applies to all the files.
	var leadingContext int64 = 0
	var trailingContext int64 = 0

	// Pre-allocate the per-filter altered check.
	// It's only used within the per-line, but it's always re-initialized on each pass.
	altered := make([]bool, parentCount)

lineLoop:
	for {
		line := p.Line(0)
		// Should be able to count the lines required by the range header,
		// however there are some rare times when that does not correctly align.
		// Therefore, the 'text.go' version of line count checking isn't used here.
		if !areValidOps(line, parentCount) {
			break
		}

		parentOps, data := line[0:parentCount], line[parentCount:]

		// Each character in parentOps is for each parent, to show how target file line
		// differs from each file of the parents.  If a fragment has a '-', then it is
		// a removal.  If another fragment has a '+' but this one has a ' ', then
		// it's also a removal.
		if parentOps == "\n" {
			// newer GNU diff versions create empty context lines
			data = "\n"
			parentOps = ""
		}

		hasAdd := false
		hasRemove := false
		hasContext := false
		for idx, op := range parentOps {
			frag := frags[idx]
			altered[idx] = false

			switch op {
			case ' ':
				// Context lines
				hasContext = true
				frag.Lines = append(frag.Lines, Line{OpContext, data})
				// Adjustment of the leading and trailing context count can only happen
				// by analyzing all the file operations, so that happens after the line's
				// operation checks.
			case '-':
				hasRemove = true
				altered[idx] = true
				noLineChanges = false
				frag.LinesDeleted++
				trailingContext = 0
				frag.Lines = append(frag.Lines, Line{OpDelete, data})
			case '+':
				hasAdd = true
				altered[idx] = true
				noLineChanges = false
				frag.LinesAdded++
				trailingContext = 0
				frag.Lines = append(frag.Lines, Line{OpAdd, data})
			case '\\':
				// this may appear in middle of fragment if it's for a deleted line
				if isNoNewlineMarker(line) {
					removeLastNewline(frag)
					// Move on to the next line.
					continue lineLoop
				}
				fallthrough
			default:
				// TODO(bkeyes): if this is because we hit the next header, it
				// would be helpful to return the miscounts line error. We could
				// either test for the common headers ("@@ -", "diff --git", "@@@ -") or
				// assume any invalid op ends the fragment; git returns the same
				// generic error in all cases so either is compatible
				return p.Errorf(0, "invalid line operation: %q", op)
			}
		}

		// The complex counting method.

		// Lines with removes reduce the old line count once per removal operation, and
		//   the counting happens during each file's removal action.
		if !hasRemove && !hasAdd && hasContext {
			// Lines with no removes, no adds, and had at least 1 context entry
			// means that this line a full context - no add and no remove.
			if noLineChanges {
				leadingContext++
			} else {
				trailingContext++
			}
		}

		if err := p.Next(); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	// There is a rare, but valid, scenario where the line counts don't match up with
	// what was parsed.  This seems related to the "-1" range value.  Because of that,
	// this function can't validate the header line count against the lines encountered.

	if noLineChanges {
		return p.Errorf(0, "fragment contains no changes")
	}

	// Check for a final "no newline" marker since it is not included in the
	// counters used to stop the loop above
	if isNoNewlineMarker(p.Line(0)) {
		for _, frag := range frags {
			removeLastNewline(frag)
		}
		if err := p.Next(); err != nil && err != io.EOF {
			return err
		}
	}

	// Because the leading and trailing context can only be determined on a whole line basis,
	// and the value can change depending on later discoveries, this count only has meaning
	// at the very end.
	for _, frag := range frags {
		frag.LeadingContext = leadingContext
		frag.TrailingContext = trailingContext
	}

	return nil
}

func areValidOps(line string, count int) bool {
	if len(line) < count {
		// Generally, this happens with an empty line ('\n'), but could also be file corruption.
		return false
	}
	for count > 0 {
		count--
		c := line[count]
		switch c {
		case ' ', '+', '-', '\\':
		default:
			return false
		}
	}
	return true
}
