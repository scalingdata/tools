// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Template support for writing HTML documents.
// Documents that include Template: true in their
// metadata are executed as input to text/template.
//
// This file defines functions for those templates to invoke.

// The template uses the function "code" to inject program
// source into the output by extracting code from files and
// injecting them as HTML-escaped <pre> blocks.
//
// The syntax is simple: 1, 2, or 3 space-separated arguments:
//
// Whole file:
//	{{code "foo.go"}}
// One line (here the signature of main):
//	{{code "foo.go" `/^func.main/`}}
// Block of text, determined by start and end (here the body of main):
//	{{code "foo.go" `/^func.main/` `/^}/`
//
// Patterns can be `/regular expression/`, a decimal number, or "$"
// to signify the end of the file. In multi-line matches,
// lines that end with the four characters
//	OMIT
// are omitted from the output, making it easy to provide marker
// lines in the input that will not appear in the output but are easy
// to identify by pattern.

package godoc

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/scalingdata/go-x-tools/godoc/vfs"
)

// Functions in this file panic on error, but the panic is recovered
// to an error by 'code'.

// contents reads and returns the content of the named file
// (from the virtual file system, so for example /doc refers to $GOROOT/doc).
func (c *Corpus) contents(name string) string {
	file, err := vfs.ReadFile(c.fs, name)
	if err != nil {
		log.Panic(err)
	}
	return string(file)
}

// stringFor returns a textual representation of the arg, formatted according to its nature.
func stringFor(arg interface{}) string {
	switch arg := arg.(type) {
	case int:
		return fmt.Sprintf("%d", arg)
	case string:
		if len(arg) > 2 && arg[0] == '/' && arg[len(arg)-1] == '/' {
			return fmt.Sprintf("%#q", arg)
		}
		return fmt.Sprintf("%q", arg)
	default:
		log.Panicf("unrecognized argument: %v type %T", arg, arg)
	}
	return ""
}

func (p *Presentation) code(file string, arg ...interface{}) (s string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	text := p.Corpus.contents(file)
	var command string
	switch len(arg) {
	case 0:
		// text is already whole file.
		command = fmt.Sprintf("code %q", file)
	case 1:
		command = fmt.Sprintf("code %q %s", file, stringFor(arg[0]))
		text = p.Corpus.oneLine(file, text, arg[0])
	case 2:
		command = fmt.Sprintf("code %q %s %s", file, stringFor(arg[0]), stringFor(arg[1]))
		text = p.Corpus.multipleLines(file, text, arg[0], arg[1])
	default:
		return "", fmt.Errorf("incorrect code invocation: code %q %q", file, arg)
	}
	// Trim spaces from output.
	text = strings.Trim(text, "\n")
	// Replace tabs by spaces, which work better in HTML.
	text = strings.Replace(text, "\t", "    ", -1)
	var buf bytes.Buffer
	// HTML-escape text and syntax-color comments like elsewhere.
	FormatText(&buf, []byte(text), -1, true, "", nil)
	// Include the command as a comment.
	text = fmt.Sprintf("<pre><!--{{%s}}\n-->%s</pre>", command, buf.Bytes())
	return text, nil
}

// parseArg returns the integer or string value of the argument and tells which it is.
func parseArg(arg interface{}, file string, max int) (ival int, sval string, isInt bool) {
	switch n := arg.(type) {
	case int:
		if n <= 0 || n > max {
			log.Panicf("%q:%d is out of range", file, n)
		}
		return n, "", true
	case string:
		return 0, n, false
	}
	log.Panicf("unrecognized argument %v type %T", arg, arg)
	return
}

// oneLine returns the single line generated by a two-argument code invocation.
func (c *Corpus) oneLine(file, text string, arg interface{}) string {
	lines := strings.SplitAfter(c.contents(file), "\n")
	line, pattern, isInt := parseArg(arg, file, len(lines))
	if isInt {
		return lines[line-1]
	}
	return lines[match(file, 0, lines, pattern)-1]
}

// multipleLines returns the text generated by a three-argument code invocation.
func (c *Corpus) multipleLines(file, text string, arg1, arg2 interface{}) string {
	lines := strings.SplitAfter(c.contents(file), "\n")
	line1, pattern1, isInt1 := parseArg(arg1, file, len(lines))
	line2, pattern2, isInt2 := parseArg(arg2, file, len(lines))
	if !isInt1 {
		line1 = match(file, 0, lines, pattern1)
	}
	if !isInt2 {
		line2 = match(file, line1, lines, pattern2)
	} else if line2 < line1 {
		log.Panicf("lines out of order for %q: %d %d", text, line1, line2)
	}
	for k := line1 - 1; k < line2; k++ {
		if strings.HasSuffix(lines[k], "OMIT\n") {
			lines[k] = ""
		}
	}
	return strings.Join(lines[line1-1:line2], "")
}

// match identifies the input line that matches the pattern in a code invocation.
// If start>0, match lines starting there rather than at the beginning.
// The return value is 1-indexed.
func match(file string, start int, lines []string, pattern string) int {
	// $ matches the end of the file.
	if pattern == "$" {
		if len(lines) == 0 {
			log.Panicf("%q: empty file", file)
		}
		return len(lines)
	}
	// /regexp/ matches the line that matches the regexp.
	if len(pattern) > 2 && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		re, err := regexp.Compile(pattern[1 : len(pattern)-1])
		if err != nil {
			log.Panic(err)
		}
		for i := start; i < len(lines); i++ {
			if re.MatchString(lines[i]) {
				return i + 1
			}
		}
		log.Panicf("%s: no match for %#q", file, pattern)
	}
	log.Panicf("unrecognized pattern: %q", pattern)
	return 0
}
