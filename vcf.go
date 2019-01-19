// Copyright 2018, 2019 Ian Johnson
//
// This file is part of vcf.
//
// Vcf is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Vcf is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with vcf. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ianprime0509/vcard"
)

const (
	defaultFormat = "%n <%e>"
	defaultInput  = "-"
)

// formatFields is a map of formatting directives to the vCard property that
// they represent.
var formatFields = map[rune]string{
	'e': "EMAIL",
	'n': "FN",
	'p': "TEL",
}

// formatField is a Value allowing formatting directives to be added via
// command-line options.
type formatField struct{}

func (formatField) String() string {
	return fmt.Sprintf("%v", formatFields)
}

func (formatField) Set(arg string) error {
	for _, field := range strings.Split(arg, ",") {
		parts := strings.Split(field, "=")
		if len(parts) != 2 || len(parts[0]) != 1 {
			return fmt.Errorf("%q is not of the format RUNE=FIELD", field)
		}
		formatFields[[]rune(parts[0])[0]] = parts[1]
	}
	return nil
}

// searchFields is a list of (optional) vCard fields to search for the user's
// search terms.
var searchFields = []string{}

// mustMatch is a set of vCard fields that must match the user's search terms
// in order for the field to be printed as a result.
var mustMatch = map[string]bool{}

// searchField is a Value allowing search fields to be added via command-line
// options.
type searchField struct{}

func (searchField) String() string {
	return fmt.Sprintf("%v", searchFields)
}

func (searchField) Set(arg string) error {
	for _, field := range strings.Split(arg, ",") {
		if len(field) == 0 {
			return errors.New("search fields cannot be empty")
		}
		if field[len(field)-1] == '!' {
			field = field[:len(field)-1]
			mustMatch[field] = true
		}
		searchFields = append(searchFields, field)
	}
	return nil
}

// escapeChars is a map of escape characters (runes) to the corresponding
// escaped rune.
var escapeChars = map[rune]rune{
	'\\': '\\',
	'0':  '\000',
	'n':  '\n',
	't':  '\t',
}

var (
	all    = flag.Bool("a", false, "include even entries with empty fields")
	format = flag.String("f", defaultFormat, "set the output format")
	input  = flag.String("i", defaultInput, "set the input file (- for stdin)")
)

func init() {
	flag.Var(formatField{}, "d", "define a formatting directive")
	flag.Var(searchField{}, "s", "add a new field to be searched")
	flag.Parse()
}

func main() {
	var inputFile io.ReadCloser
	if *input == "-" {
		inputFile = os.Stdin
	} else {
		var err error
		inputFile, err = os.Open(*input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vcf: could not open input file: %v\n", err)
			os.Exit(1)
		}
		defer inputFile.Close()
	}

	err := run(os.Stdout, inputFile, flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "vcf: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main program logic (reading, formatting and writing output)
// using the given writer, reader and search terms.
func run(w io.Writer, r io.Reader, search []string) error {
	*format = unescape(*format)
	p := vcard.NewParser(bufio.NewReader(r))

	card, err := p.Next()
	for err == nil {
		if matchesSearch(card, search) {
			err := formatCard(w, card, search)
			if err != nil {
				return err
			}
		}
		card, err = p.Next()
	}

	if err != io.EOF {
		return fmt.Errorf("could not read card: %v", err)
	}
	return nil
}

// matchesSearch returns whether the given vCard matches the given
// (case-insensitive) search terms. As a special case, if no search terms are
// given, the result is always true.
func matchesSearch(card *vcard.Card, search []string) bool {
	if len(search) == 0 {
		return true
	}
	for _, term := range search {
		term = strings.ToUpper(term)
		for _, field := range searchFields {
			props := card.Get(field)
			for _, prop := range props {
				if propMatchesTerm(prop, term) {
					goto nextTerm
				}
			}
		}
		// We got through all available field values and none matched.
		return false
	nextTerm:
	}
	return true
}

// propMatchesSearch returns whether the given vCard property matches at least
// one of the given (case-insensitive) search terms. As a special case, if no
// search terms are given, the result is always true.
func propMatchesSearch(prop vcard.Property, search []string) bool {
	if len(search) == 0 {
		return true
	}
	for _, term := range search {
		term = strings.ToUpper(term)
		if propMatchesTerm(prop, term) {
			return true
		}
	}
	return false
}

// propMatchesTerm returns whether the given vCard property matches the given
// (case-sensitive) search term.
func propMatchesTerm(prop vcard.Property, term string) bool {
	for _, value := range prop.Values() {
		value = strings.ToUpper(value)
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

// formatCard formats the given vCard according to the format string specified
// in the command-line arguments, also taking into account other options such
// as "-a". The search query is also provided, in case any properties are
// marked as "must match".
func formatCard(w io.Writer, card *vcard.Card, search []string) error {
	// In order to handle cards that may have more than one of each field,
	// we need to maintain several strings, which will become the lines of
	// the output. Every time we get a field repeated n times, we need to
	// obtain (n-1) copies of each string under construction and use all
	// of them for further operations.
	//
	// We use a bytes.Buffer instead of a strings.Builder because the
	// latter doesn't like being copied; even though we aren't actually
	// doing any unsafe copying, the slice might get copied somewhere else
	// during an append, which would make the strings.Buffer complain.
	out := make([]*bytes.Buffer, 1)
	out[0] = new(bytes.Buffer)

	inFormat := false    // whether we're processing a formatting directive
	modifier := rune(-1) // the modifier for this formatting directive
	for _, r := range *format {
		if r == '%' {
			if inFormat {
				for _, b := range out {
					b.WriteRune('%')
				}
				inFormat = false
			} else {
				inFormat = true
				modifier = -1
			}
		} else if inFormat {
			if r == '+' {
				if modifier != -1 {
					return fmt.Errorf("already using modifier %q", modifier)
				}
				modifier = r
				continue
			}
			field, ok := formatFields[r]
			if !ok {
				return fmt.Errorf("unknown formatting directive %q", r)
			}

			props := card.Get(field)
			if mustMatch[field] {
				props = filterProps(props, search)
			}
			if len(props) == 0 && !*all {
				// Break out of the function early without
				// printing anything for this card.
				return nil
			}
			appendProps(&out, props, modifier)

			inFormat = false
		} else {
			for _, b := range out {
				b.WriteRune(r)
			}
		}
	}

	if inFormat {
		return errors.New("unexpected end of format string")
	}

	for _, b := range out {
		fmt.Fprintln(w, b)
	}
	return nil
}

// filterProps returns only those properties from the given slice that match
// one or more of the given search terms. As a special case, if no search terms
// are given, all properties are returned.
func filterProps(props []vcard.Property, search []string) []vcard.Property {
	if len(search) == 0 {
		return props
	}
	filtered := make([]vcard.Property, 0, len(props))
	for _, prop := range props {
		if propMatchesSearch(prop, search) {
			filtered = append(filtered, prop)
		}
	}
	return filtered
}

// unescape transforms escaped characters in the given string to their unescaped
// forms. Unknown escape characters result in themselves (e.g. "\s" becomes "s"),
// and a trailing backslash is lost (e.g. "hello\" becomes "hello").
func unescape(s string) string {
	sb := new(strings.Builder)
	inEscape := false
	for _, r := range s {
		if r == '\\' && !inEscape {
			inEscape = true
		} else if inEscape {
			unescaped, ok := escapeChars[r]
			if !ok {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(unescaped)
			}
			inEscape = false
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// appendProps appends the given properties to all the buffers in the given
// slice, creating new buffers if necessary such that the result is the Cartesian
// product of the previous buffers and the given properties.
func appendProps(bufs *[]*bytes.Buffer, props []vcard.Property, mod rune) {
	// Handle possible copies first.
	oldBufs := *bufs
	if len(props) > 1 {
		for _, prop := range props[1:] {
			for _, ob := range oldBufs {
				bs := make([]byte, ob.Len())
				copy(bs, ob.Bytes())
				nb := bytes.NewBuffer(bs)
				nb.WriteString(formatProp(prop, mod))
				*bufs = append(*bufs, nb)
			}
		}
	}
	if len(props) > 0 {
		for _, b := range (*bufs)[:len(oldBufs)] {
			b.WriteString(formatProp(props[0], mod))
		}
	}
}

// formatProp formats the given property, taking into account the given
// formatting modifier.
func formatProp(prop vcard.Property, mod rune) string {
	// TODO: figure out a better way to handle multiple values.
	base := strings.Join(prop.Values(), ",")
	if mod == '+' {
		return quoteCSV(base)
	}
	return base
}

// quoteCSV quotes the given string according to CSV quoting rules. This means
// that the string will be surrounded with double quotes, and any double quotes
// present in the string will be doubled (to disambiguate them from the
// surrounding quotes).
func quoteCSV(s string) string {
	sb := new(strings.Builder)
	sb.WriteRune('"')
	for _, r := range s {
		if r == '"' {
			sb.WriteString("\"\"")
		} else {
			sb.WriteRune(r)
		}
	}
	sb.WriteRune('"')
	return sb.String()
}
