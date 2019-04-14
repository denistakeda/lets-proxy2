// Copyright (c) 2018, Maxime Soulé
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package ctxerr

import (
	"bytes"
	"os"
	"strings"

	"github.com/maxatome/go-testdeep/internal/location"
	"github.com/maxatome/go-testdeep/internal/util"
)

const (
	envColor      = "TESTDEEP_COLOR"
	envColorOK    = "TESTDEEP_COLOR_OK"
	envColorBad   = "TESTDEEP_COLOR_BAD"
	envColorTitle = "TESTDEEP_COLOR_TITLE"
)

var (
	_, colorTitleOn, colorTitleOff          = colorFromEnv(envColorTitle, "cyan")
	colorOKOn, colorOKOnBold, colorOKOff    = colorFromEnv(envColorOK, "green")
	colorBadOn, colorBadOnBold, colorBadOff = colorFromEnv(envColorBad, "red")
)

var colors = map[string]byte{
	"black":   '0',
	"red":     '1',
	"green":   '2',
	"yellow":  '3',
	"blue":    '4',
	"magenta": '5',
	"cyan":    '6',
	"white":   '7',
	"gray":    '7',
}

func colorFromEnv(env, defaultColor string) (string, string, string) {
	var color string
	switch os.Getenv(envColor) {
	case "on", "":
		if curColor := os.Getenv(env); curColor != "" {
			color = curColor
		} else {
			color = defaultColor
		}
	default: // "off" or any other value
		color = ""
	}

	if color == "" {
		return "", "", ""
	}

	names := strings.SplitN(color, ":", 2)

	light := [...]byte{
		//   0    1    2    4    4    5    6
		'\x1b', '[', '0', ';', '3', 'y', 'm', // foreground
		//   7    8    9   10   11
		'\x1b', '[', '4', 'z', 'm', // background
	}
	bold := [...]byte{
		//   0    1    2    4    4    5    6
		'\x1b', '[', '1', ';', '3', 'y', 'm', // foreground
		//   7    8    9   10   11
		'\x1b', '[', '4', 'z', 'm', // background
	}

	var start, end int

	// Foreground
	if names[0] != "" {
		c := colors[names[0]]
		if c == 0 {
			c = colors[defaultColor]
		}

		light[5] = c
		bold[5] = c

		end = 7
	} else {
		start = 7
	}

	// Background
	if len(names) > 1 && names[1] != "" {
		c := colors[names[1]]
		if c != 0 {
			light[10] = c
			bold[10] = c

			end = 12
		}
	}

	return string(light[start:end]), string(bold[start:end]), "\x1b[0m"
}

// Error represents errors generated by testdeep functions.
type Error struct {
	// Context when the error occurred
	Context Context
	// Message describes the error
	Message string
	// Got value
	Got interface{}
	// Expected value
	Expected interface{}
	// If not nil, Summary is used to display summary instead of using
	// Got + Expected fields
	Summary interface{}
	// If initialized, location of TestDeep operator originator of the error
	Location location.Location
	// If defined, the current Error comes from this Error
	Origin *Error
	// If defined, points to the next Error
	Next *Error
}

var (
	// BooleanError is the *Error returned when an error occurs in a
	// boolean context.
	BooleanError = &Error{}

	// ErrTooManyErrors is chained to the last error encountered when
	// the maximum number of errors has been reached.
	ErrTooManyErrors = &Error{
		Message: "Too many errors (use TESTDEEP_MAX_ERRORS=-1 to see all)",
	}
)

// Error implements error interface.
func (e *Error) Error() string {
	buf := bytes.Buffer{}

	e.Append(&buf, "")

	return buf.String()
}

// Append appends the Error contents to "buf" using prefix "prefix"
// for each line.
func (e *Error) Append(buf *bytes.Buffer, prefix string) {
	if e == BooleanError {
		return
	}

	var writeEolPrefix func()
	if prefix != "" {
		eolPrefix := make([]byte, 1+len(prefix))
		eolPrefix[0] = '\n'
		copy(eolPrefix[1:], prefix)

		writeEolPrefix = func() {
			buf.Write(eolPrefix)
		}
		buf.WriteString(prefix)
	} else {
		writeEolPrefix = func() {
			buf.WriteByte('\n')
		}
	}

	if e == ErrTooManyErrors {
		buf.WriteString(colorTitleOn)
		buf.WriteString(e.Message)
		buf.WriteString(colorTitleOff)
		return
	}

	buf.WriteString(colorTitleOn)
	if pos := strings.Index(e.Message, "%%"); pos >= 0 {
		buf.WriteString(e.Message[:pos])
		buf.WriteString(e.Context.Path)
		buf.WriteString(e.Message[pos+2:])
	} else {
		buf.WriteString(e.Context.Path)
		buf.WriteString(": ")
		buf.WriteString(e.Message)
	}
	buf.WriteString(colorTitleOff)

	writeEolPrefix()

	if e.Summary != nil {
		buf.WriteByte('\t')
		buf.WriteString(colorBadOn)
		buf.WriteString(util.IndentString(e.SummaryString(), prefix+"\t"))
		buf.WriteString(colorBadOff)
	} else {
		buf.WriteString(colorBadOnBold)
		buf.WriteString("\t     got: ")
		buf.WriteString(colorBadOn)
		buf.WriteString(util.IndentString(e.GotString(), prefix+"\t          "))
		buf.WriteString(colorBadOff)
		writeEolPrefix()
		buf.WriteString(colorOKOnBold)
		buf.WriteString("\texpected: ")
		buf.WriteString(colorOKOn)
		buf.WriteString(util.IndentString(e.ExpectedString(), prefix+"\t          "))
		buf.WriteString(colorOKOff)
	}

	// This error comes from another one
	if e.Origin != nil {
		writeEolPrefix()
		buf.WriteString("Originates from following error:\n")

		e.Origin.Append(buf, prefix+"\t")
	}

	if e.Location.IsInitialized() &&
		!strings.HasPrefix(e.Location.Func, "Cmp") && // no need to log Cmp* func
		(e.Next == nil || e.Next.Location != e.Location) {
		writeEolPrefix()
		buf.WriteString("[under TestDeep operator ")
		buf.WriteString(e.Location.String())
		buf.WriteByte(']')
	}

	if e.Next != nil {
		buf.WriteByte('\n')
		e.Next.Append(buf, prefix) // next error at same level
	}
}

// GotString returns the string corresponding to the Got
// field. Returns the empty string if the Error Summary field is not
// empty.
func (e *Error) GotString() string {
	if e.Summary != nil {
		return ""
	}
	return util.ToString(e.Got)
}

// ExpectedString returns the string corresponding to the Expected
// field. Returns the empty string if the Error Summary field is not
// empty.
func (e *Error) ExpectedString() string {
	if e.Summary != nil {
		return ""
	}
	return util.ToString(e.Expected)
}

// SummaryString returns the string corresponding to the Summary
// field. Returns the empty string if the Error Summary field is nil.
func (e *Error) SummaryString() string {
	if e.Summary == nil {
		return ""
	}
	return util.ToString(e.Summary)
}
