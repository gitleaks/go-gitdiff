package gitdiff

import (
	"io"
	"reflect"
	"testing"
)

func TestParseCombinedTextFragmentHeader(t *testing.T) {
	tests := map[string]struct {
		Input  string
		Output []TextFragment
		Err    string
	}{
		"shortest": {
			Input: "@@@ -1 -1 +1 @@@\n",
			Output: []TextFragment{{
				OldPosition: 1,
				OldLines:    1,
				NewPosition: 1,
				NewLines:    1,
			}, {
				OldPosition: 1,
				OldLines:    1,
				NewPosition: 1,
				NewLines:    1,
			}},
		},
		"twoParent": {
			Input: "@@@ -52,1 -50,2 +52,2 @@@\n",
			Output: []TextFragment{{
				OldPosition: 52,
				OldLines:    1,
				NewPosition: 52,
				NewLines:    2,
			}, {
				OldPosition: 50,
				OldLines:    2,
				NewPosition: 52,
				NewLines:    2,
			}},
		},
		"threeParent": {
			Input: "@@@@ -52,1 -50,2 -38,5 +42,9 @@@@\n",
			Output: []TextFragment{{
				OldPosition: 52,
				OldLines:    1,
				NewPosition: 42,
				NewLines:    9,
			}, {
				OldPosition: 50,
				OldLines:    2,
				NewPosition: 42,
				NewLines:    9,
			}, {
				OldPosition: 38,
				OldLines:    5,
				NewPosition: 42,
				NewLines:    9,
			}},
		},
		"trailingComment": {
			Input: "@@@ -52,1 -50,2 +52,2 @@@ func test(n int) {\n",
			Output: []TextFragment{{
				Comment:     "func test(n int) {",
				OldPosition: 52,
				OldLines:    1,
				NewPosition: 52,
				NewLines:    2,
			}, {
				Comment:     "func test(n int) {",
				OldPosition: 50,
				OldLines:    2,
				NewPosition: 52,
				NewLines:    2,
			}},
		},
		"negativeCount2": {
			Input: "@@@ -229,4 -229,18446744073709551615 +228,3 @@@ Comment\n",
			Output: []TextFragment{{
				Comment:     "Comment",
				OldPosition: 229,
				OldLines:    5,
				NewPosition: 228,
				NewLines:    4,
			}, {
				Comment:     "Comment",
				OldPosition: 229,
				OldLines:    0,
				NewPosition: 228,
				NewLines:    4,
			}},
		},
		"negativeCount3": {
			Input: "@@@ -229,4 -229,1 +228,18446744073709551615 @@@ Comment\n",
			Output: []TextFragment{{
				Comment:     "Comment",
				OldPosition: 229,
				OldLines:    5,
				NewPosition: 228,
				NewLines:    0,
			}, {
				Comment:     "Comment",
				OldPosition: 229,
				OldLines:    2,
				NewPosition: 228,
				NewLines:    0,
			}},
		},
		"incompleteThree": {
			Input: "@@@ -12,3 -5,9 +2\n",
			Err:   "gitdiff: line 1: invalid fragment header",
		},
		"incompleteTwo": {
			Input: "@@@ -12,3 +2,5\n",
			Err:   "gitdiff: line 1: invalid fragment header",
		},
		"incompleteFour": {
			Input: "@@@@ -12,3 -5,9 -2,6\n",
			Err:   "gitdiff: line 1: invalid fragment header",
		},
		"incompleteFourDiffs": {
			Input: "@@@@ -12,3 -5,9 -2,6 @@@@\n",
			Err:   "gitdiff: line 1: invalid fragment header",
		},
		"badNumbers": {
			Input: "@@@ -1a,2b -3c,4d +5e,6f @@@\n",
			Err:   "gitdiff: line 1: invalid fragment header: bad start of range: 5e: invalid syntax",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := newTestParser(test.Input, true)

			frags, err := p.ParseCombinedTextFragmentHeader()
			if test.Err != "" {
				if err == nil || err == io.EOF {
					t.Fatalf("expected error parsing header, but got %v", err)
				}
				if test.Err != err.Error() {
					t.Errorf("incorrect error: expected %s actual %v", test.Err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error parsing header: %v", err)
			}

			if !reflect.DeepEqual(joinFragmentPtr(test.Output), frags) {
				t.Errorf("incorrect fragment\nexpected: %+v\nactual: %+v", test.Output, reduceFragmentPtr(frags))
			}
		})
	}
}

func TestCombinedParseTextChunk(t *testing.T) {
	tests := map[string]struct {
		Input     string
		Fragments []TextFragment

		Output []TextFragment
		Err    bool
	}{
		"removeAddRemoveDAdd": {
			Input: `- old line 1.0
+ new line 1.1
 -old line 2.1
++new line 1.2,2.2
`,
			Fragments: []TextFragment{
				{OldLines: 1, NewLines: 2},
				{OldLines: 2, NewLines: 2},
			},
			Output: []TextFragment{
				{
					OldLines: 1,
					NewLines: 2,
					Lines: []Line{
						{OpDelete, "old line 1.0\n"},
						{OpAdd, "new line 1.1\n"},
						{OpContext, "old line 2.1\n"},
						{OpAdd, "new line 1.2,2.2\n"},
					},
					LinesAdded:      2,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					OldLines: 2,
					NewLines: 2,
					Lines: []Line{
						{OpContext, "old line 1.0\n"},
						{OpContext, "new line 1.1\n"},
						{OpDelete, "old line 2.1\n"},
						{OpAdd, "new line 1.2,2.2\n"},
					},
					LinesAdded:      1,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
		"addRemoveAddRemove": {
			Input: ` -remove line 1.1
 +add line 1.2
- remove line 2.1
+ add line 2.2
`,
			Fragments: []TextFragment{
				{OldLines: 2, NewLines: 2},
				{OldLines: 2, NewLines: 2},
			},
			Output: []TextFragment{
				{
					OldLines: 2,
					NewLines: 2,
					Lines: []Line{
						{OpContext, "remove line 1.1\n"},
						{OpContext, "add line 1.2\n"},
						{OpDelete, "remove line 2.1\n"},
						{OpAdd, "add line 2.2\n"},
					},
					LinesAdded:      1,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					OldLines: 2,
					NewLines: 2,
					Lines: []Line{
						{OpDelete, "remove line 1.1\n"},
						{OpAdd, "add line 1.2\n"},
						{OpContext, "remove line 2.1\n"},
						{OpContext, "add line 2.2\n"},
					},
					LinesAdded:      1,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
		"removeAdd3RemoveAdd": {
			Input: `- remove line 1.1
+ add line 1.2
 -remove line 2.1
 -remove line 2.2
 -remove line 2.3
 +add line 2.4
 `,
			Fragments: []TextFragment{
				{OldLines: 2, NewLines: 2},
				{OldLines: 4, NewLines: 2},
			},
			Output: []TextFragment{
				{
					OldLines: 2,
					NewLines: 2,
					Lines: []Line{
						{OpDelete, "remove line 1.1\n"},
						{OpAdd, "add line 1.2\n"},
						{OpContext, "remove line 2.1\n"},
						{OpContext, "remove line 2.2\n"},
						{OpContext, "remove line 2.3\n"},
						{OpContext, "add line 2.4\n"},
					},
					LinesAdded:      1,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					OldLines: 4,
					NewLines: 2,
					Lines: []Line{
						{OpContext, "remove line 1.1\n"},
						{OpContext, "add line 1.2\n"},
						{OpDelete, "remove line 2.1\n"},
						{OpDelete, "remove line 2.2\n"},
						{OpDelete, "remove line 2.3\n"},
						{OpAdd, "add line 2.4\n"},
					},
					LinesAdded:      1,
					LinesDeleted:    3,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
		"bothChangeEol": {
			Input: `- remove line 1.1
 -remove line 2.1
++add line 1.1,2.2`,
			Fragments: []TextFragment{
				{OldLines: 1, NewLines: 1},
				{OldLines: 1, NewLines: 1},
			},
			Output: []TextFragment{
				{
					OldLines: 1,
					NewLines: 1,
					Lines: []Line{
						{OpDelete, "remove line 1.1\n"},
						{OpContext, "remove line 2.1\n"},
						{OpAdd, "add line 1.1,2.2"},
					},
					LinesAdded:      1,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					OldLines: 1,
					NewLines: 1,
					Lines: []Line{
						{OpContext, "remove line 1.1\n"},
						{OpDelete, "remove line 2.1\n"},
						{OpAdd, "add line 1.1,2.2"},
					},
					LinesAdded:      1,
					LinesDeleted:    1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
		"doubleRemove": {
			Input: `--line 1.1,2.1
- line 1.2
 -line 2.2
`,
			Fragments: []TextFragment{
				{OldLines: 2, NewLines: 0},
				{OldLines: 2, NewLines: 0},
			},
			Output: []TextFragment{
				{
					OldLines: 2,
					NewLines: 0,
					Lines: []Line{
						{OpDelete, "line 1.1,2.1\n"},
						{OpDelete, "line 1.2\n"},
						{OpContext, "line 2.2\n"},
					},
					LinesAdded:      0,
					LinesDeleted:    2,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					OldLines: 2,
					NewLines: 0,
					Lines: []Line{
						{OpDelete, "line 1.1,2.1\n"},
						{OpContext, "line 1.2\n"},
						{OpDelete, "line 2.2\n"},
					},
					LinesAdded:      0,
					LinesDeleted:    2,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
		"doubleAddAdd": {
			Input: `++line 1.1,2.1
+ line 1.2
`,
			Fragments: []TextFragment{
				{OldLines: 0, NewLines: 2},
				{OldLines: 1, NewLines: 2},
			},
			Output: []TextFragment{
				{
					OldLines: 0,
					NewLines: 2,
					Lines: []Line{
						{OpAdd, "line 1.1,2.1\n"},
						{OpAdd, "line 1.2\n"},
					},
					LinesAdded:      2,
					LinesDeleted:    0,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					OldLines: 1,
					NewLines: 2,
					Lines: []Line{
						{OpAdd, "line 1.1,2.1\n"},
						{OpContext, "line 1.2\n"},
					},
					LinesAdded:      1,
					LinesDeleted:    0,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := newTestParser(test.Input, true)

			frags := joinFragmentPtr(test.Fragments)
			err := p.ParseCombinedTextChunk(frags)
			if test.Err {
				if err == nil || err == io.EOF {
					t.Fatalf("expected error parsing text chunk, but got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error parsing text chunk: %v", err)
			}

			if !reflect.DeepEqual(test.Output, reduceFragmentPtr(frags)) {
				t.Errorf("incorrect fragment\nexpected: %+v\nactual: %+v", test.Output, reduceFragmentPtr(frags))
			}
		})
	}
}

func TestCombinedParseTextFragments(t *testing.T) {
	tests := map[string]struct {
		Input string
		File  File

		Fragments []TextFragment
		Err       string
	}{
		"multipleChanges": {
			Input: `@@@ -54,0 -52,2 +51,3 @@@ file 1
++line 1.1.1,1.2.1
+ line 1.1.1
+ line 1.1.2
@@@ -134,1 -136,1 +136,0 @@@ file 2
- line 2.1.1
 -line 2.2.1
`,
			Fragments: []TextFragment{
				{
					Comment:     "file 1",
					OldPosition: 54,
					OldLines:    0,
					NewPosition: 51,
					NewLines:    3,
					Lines: []Line{
						{OpAdd, "line 1.1.1,1.2.1\n"},
						{OpAdd, "line 1.1.1\n"},
						{OpAdd, "line 1.1.2\n"},
					},
					LinesAdded:      3,
					LinesDeleted:    0,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					Comment:     "file 1",
					OldPosition: 52,
					OldLines:    2,
					NewPosition: 51,
					NewLines:    3,
					Lines: []Line{
						{OpAdd, "line 1.1.1,1.2.1\n"},
						{OpContext, "line 1.1.1\n"},
						{OpContext, "line 1.1.2\n"},
					},
					LinesDeleted:    0,
					LinesAdded:      1,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					Comment:     "file 2",
					OldPosition: 134,
					OldLines:    1,
					NewPosition: 136,
					NewLines:    0,
					Lines: []Line{
						{OpDelete, "line 2.1.1\n"},
						{OpContext, "line 2.2.1\n"},
					},
					LinesDeleted:    1,
					LinesAdded:      0,
					LeadingContext:  0,
					TrailingContext: 0,
				},
				{
					Comment:     "file 2",
					OldPosition: 136,
					OldLines:    1,
					NewPosition: 136,
					NewLines:    0,
					Lines: []Line{
						{OpContext, "line 2.1.1\n"},
						{OpDelete, "line 2.2.1\n"},
					},
					LinesDeleted:    1,
					LinesAdded:      0,
					LeadingContext:  0,
					TrailingContext: 0,
				},
			},
		},
		"strangeCount": {
			// This test is why the implementation can't rely upon the lines changed.
			// Its values are off by 1, according to the description of the meaning in the
			// documentation.
			Input: `@@@ -225,4 -237,2 +225,6 @@@ C0
 +line 1
 +line 2
 +line 3
 +line 4
- line 5
++line 6
++line 7
+ line 8
 -line 9
 -line 10
`,
			Fragments: []TextFragment{
				{
					Comment:     "C0",
					OldPosition: 225,
					OldLines:    4,
					NewPosition: 225,
					NewLines:    6,
					Lines: []Line{
						{OpContext, "line 1\n"},
						{OpContext, "line 2\n"},
						{OpContext, "line 3\n"},
						{OpContext, "line 4\n"},
						{OpDelete, "line 5\n"},
						{OpAdd, "line 6\n"},
						{OpAdd, "line 7\n"},
						{OpAdd, "line 8\n"},
						{OpContext, "line 9\n"},
						{OpContext, "line 10\n"},
					},
					LinesDeleted: 1,
					LinesAdded:   3,
				},
				{
					Comment:     "C0",
					OldPosition: 237,
					OldLines:    2,
					NewPosition: 225,
					NewLines:    6,
					Lines: []Line{
						{OpAdd, "line 1\n"},
						{OpAdd, "line 2\n"},
						{OpAdd, "line 3\n"},
						{OpAdd, "line 4\n"},
						{OpContext, "line 5\n"},
						{OpAdd, "line 6\n"},
						{OpAdd, "line 7\n"},
						{OpContext, "line 8\n"},
						{OpDelete, "line 9\n"},
						{OpDelete, "line 10\n"},
					},
					LinesDeleted: 2,
					LinesAdded:   6,
				},
			},
		},
		"negativeLines1": {
			Input: `@@@ -229,4 -230,18446744073709551615 +228,3 @@@ C1
 +line 1
 +line 2
 +line 3
 +line 4
- line 5
`,
			Fragments: []TextFragment{
				{
					Comment:     "C1",
					OldPosition: 229,
					OldLines:    5,
					NewPosition: 228,
					NewLines:    4,
					Lines: []Line{
						{OpContext, "line 1\n"},
						{OpContext, "line 2\n"},
						{OpContext, "line 3\n"},
						{OpContext, "line 4\n"},
						{OpDelete, "line 5\n"},
					},
					LinesDeleted: 1,
					LinesAdded:   0,
				},
				{
					Comment:     "C1",
					OldPosition: 230,
					OldLines:    0,
					NewPosition: 228,
					NewLines:    4,
					Lines: []Line{
						{OpAdd, "line 1\n"},
						{OpAdd, "line 2\n"},
						{OpAdd, "line 3\n"},
						{OpAdd, "line 4\n"},
						{OpContext, "line 5\n"},
					},
					LinesDeleted: 0,
					LinesAdded:   4,
				},
			},
		},
		"negativeLines2": {
			Input: `@@@ -232,18446744073709551615 -227,1 +230,1 @@@
++line 1
+ line 2
 -line 3
`,
			Fragments: []TextFragment{
				{
					OldPosition: 232,
					OldLines:    0,
					NewPosition: 230,
					NewLines:    2,
					Lines: []Line{
						{OpAdd, "line 1\n"},
						{OpAdd, "line 2\n"},
						{OpContext, "line 3\n"},
					},
					LinesDeleted: 0,
					LinesAdded:   2,
				},
				{
					OldPosition: 227,
					OldLines:    2,
					NewPosition: 230,
					NewLines:    2,
					Lines: []Line{
						{OpAdd, "line 1\n"},
						{OpContext, "line 2\n"},
						{OpDelete, "line 3\n"},
					},
					LinesDeleted: 1,
					LinesAdded:   1,
				},
			},
		},
		"negativeMergedLines": {
			Input: `@@@ -1,2 -1,2 +1,18446744073709551615 @@@
--line 1
--line 2
--line 3
`,
			File: File{IsDelete: true},
			Fragments: []TextFragment{
				{
					OldPosition: 1,
					OldLines:    3,
					NewPosition: 1,
					NewLines:    0,
					Lines: []Line{
						{OpDelete, "line 1\n"},
						{OpDelete, "line 2\n"},
						{OpDelete, "line 3\n"},
					},
					LinesDeleted: 3,
					LinesAdded:   0,
				},
				{
					OldPosition: 1,
					OldLines:    3,
					NewPosition: 1,
					NewLines:    0,
					Lines: []Line{
						{OpDelete, "line 1\n"},
						{OpDelete, "line 2\n"},
						{OpDelete, "line 3\n"},
					},
					LinesDeleted: 3,
					LinesAdded:   0,
				},
			},
		},
		"badNewFile": {
			Input: `@@@@ -1 -1 -1 +1,2 @@@@
---old line 1
+  new line 1
+  new line 2
			`,
			File: File{
				IsNew: true,
			},
			Err: "gitdiff: line 1: new file depends on old contents",
		},
		"badDeletedFile": {
			Input: `@@@ -1,2 -1 +1 @@@
--old line 1
  context line
			`,
			File: File{
				IsDelete: true,
			},
			Err: "gitdiff: line 1: deleted file still has contents",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := newTestParser(test.Input, true)

			file := test.File
			n, err := p.ParseCombinedTextFragments(&file)
			if test.Err != "" {
				if err == nil || err == io.EOF {
					t.Fatalf("expected error parsing text fragments, but returned %v", err)
				}
				if test.Err != err.Error() {
					t.Fatalf("Incorrect error text: expected %v, actual %v", test.Err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error parsing text fragments: %v", err)
			}

			if len(test.Fragments) != n {
				t.Fatalf("incorrect number of added fragments: expected %d, actual %d", len(test.Fragments), n)
			}

			for i, frag := range test.Fragments {
				if !reflect.DeepEqual(&frag, file.TextFragments[i]) {
					t.Errorf("incorrect fragment at position %d\nexpected: %+v\nactual: %+v", i, frag, *file.TextFragments[i])
				}
			}
		})
	}
}

func joinFragmentPtr(frags []TextFragment) []*TextFragment {
	ret := make([]*TextFragment, len(frags))
	for i, f := range frags {
		r := copyFragment(&f)
		ret[i] = &r
	}
	return ret
}

func reduceFragmentPtr(frags []*TextFragment) []TextFragment {
	ret := make([]TextFragment, len(frags))
	for i, f := range frags {
		r := copyFragment(f)
		ret[i] = r
	}
	return ret
}

func copyFragment(f *TextFragment) TextFragment {
	return TextFragment{
		Comment:         f.Comment,
		OldPosition:     f.OldPosition,
		OldLines:        f.OldLines,
		NewPosition:     f.NewPosition,
		NewLines:        f.NewLines,
		LinesAdded:      f.LinesAdded,
		LinesDeleted:    f.LinesDeleted,
		LeadingContext:  f.LeadingContext,
		TrailingContext: f.TrailingContext,
		Lines:           f.Lines,
	}
}
