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

var (
	format *string
	input  *string
)

func init() {
	format = flag.String("f", defaultFormat, "set the output format")
	input = flag.String("i", defaultInput, "set the input file (- for stdin)")
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
			fmt.Fprintf(os.Stderr, "contact: could not open input file: %v\n", err)
			os.Exit(1)
		}
		defer inputFile.Close()
	}

	search := strings.Join(flag.Args(), " ")
	err := run(os.Stdout, inputFile, *format, search)
	if err != nil {
		fmt.Fprintf(os.Stderr, "contact: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main program logic (reading, formatting and writing output)
// using the given reader, format string, search string and writer.
func run(w io.Writer, r io.Reader, format, search string) error {
	p := vcard.NewParser(bufio.NewReader(r))

	card, err := p.Next()
	for err == nil {
		if matchesSearch(card, search) {
			err := formatCard(w, card, format)
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

// matchesSearch returns whether the given vCard matches the given search query.
func matchesSearch(card *vcard.Card, search string) bool {
	return true
}

// formatCard formats the given vCard according to a format string.
func formatCard(w io.Writer, card *vcard.Card, format string) error {
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

	inFormat := false // whether we're processing a formatting directive
	for _, r := range format {
		if r == '%' {
			if inFormat {
				for _, b := range out {
					b.WriteRune('%')
				}
				inFormat = false
			} else {
				inFormat = true
			}
		} else if inFormat {
			field, ok := formatFields[r]
			if !ok {
				return fmt.Errorf("unknown formatting directive %q", r)
			}
			props := card.Get(field)

			// Handle possible copies first.
			oldOut := out
			if len(props) > 1 {
				for _, prop := range props[1:] {
					for _, ob := range oldOut {
						bs := make([]byte, ob.Len())
						copy(bs, ob.Bytes())
						nb := bytes.NewBuffer(bs)
						// TODO: figure out a better way to handle multiple values.
						nb.WriteString(strings.Join(prop.Values(), ","))
						out = append(out, nb)
					}
				}
			}
			if len(props) > 0 {
				for _, b := range out[:len(oldOut)] {
					b.WriteString(strings.Join(props[0].Values(), ","))
				}
			}

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
