// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmt

import (
	"strconv"
	"unicode/utf8"
)

const (
	// %b of an int64, plus a sign.
	// Hex can add 0x and we handle it specially.
	nByte = 65

	ldigits = "0123456789abcdefx"
	udigits = "0123456789ABCDEFX"
)

const (
	signed   = true
	unsigned = false
)

// flags placed in a separate struct for easy clearing.
type fmtFlags struct {
	widPresent  bool
	precPresent bool
	minus       bool
	plus        bool
	sharp       bool
	space       bool
	zero        bool

	// For the formats %+v %#v, we set the plusV/sharpV flags
	// and clear the plus/sharp flags since %+v and %#v are in effect
	// different, flagless formats set at the top level.
	plusV  bool
	sharpV bool
}

// A fmt is the raw formatter used by Printf etc.
// It prints into a buffer that must be set up separately.
type fmt struct {
	intbuf [nByte]byte
	buf    *buffer
	// width, precision
	wid  int
	prec int
	fmtFlags
}

func (f *fmt) clearflags() {
	f.fmtFlags = fmtFlags{}
}

func (f *fmt) init(buf *buffer) {
	f.buf = buf
	f.clearflags()
}

// writePadding generates n bytes of padding.
func (f *fmt) writePadding(n int) {
	if n <= 0 { // No padding bytes needed.
		return
	}
	buf := *f.buf
	oldLen := len(buf)
	newLen := oldLen + n
	// Make enough room for padding.
	if newLen > cap(buf) {
		buf = make(buffer, cap(buf)*2+n)
		copy(buf, *f.buf)
	}
	// Decide which byte the padding should be filled with.
	padByte := byte(' ')
	if f.zero {
		padByte = byte('0')
	}
	// Fill padding with padByte.
	padding := buf[oldLen:newLen]
	for i := range padding {
		padding[i] = padByte
	}
	*f.buf = buf[:newLen]
}

// pad appends b to f.buf, padded on left (!f.minus) or right (f.minus).
func (f *fmt) pad(b []byte) {
	if !f.widPresent || f.wid == 0 {
		f.buf.Write(b)
		return
	}
	width := f.wid - utf8.RuneCount(b)
	if !f.minus {
		// left padding
		f.writePadding(width)
		f.buf.Write(b)
	} else {
		// right padding
		f.buf.Write(b)
		f.writePadding(width)
	}
}

// padString appends s to f.buf, padded on left (!f.minus) or right (f.minus).
func (f *fmt) padString(s string) {
	if !f.widPresent || f.wid == 0 {
		f.buf.WriteString(s)
		return
	}
	width := f.wid - utf8.RuneCountInString(s)
	if !f.minus {
		// left padding
		f.writePadding(width)
		f.buf.WriteString(s)
	} else {
		// right padding
		f.buf.WriteString(s)
		f.writePadding(width)
	}
}

// fmt_boolean formats a boolean.
func (f *fmt) fmt_boolean(v bool) {
	if v {
		f.padString("true")
	} else {
		f.padString("false")
	}
}

// fmt_unicode formats a uint64 as "U+0078" or with f.sharp set as "U+0078 'x'".
func (f *fmt) fmt_unicode(u uint64) {
	buf := f.intbuf[0:]

	// With default precision set the maximum needed buf length is 18
	// for formatting -1 with %#U ("U+FFFFFFFFFFFFFFFF")
	// which fits into the already allocated intbuf with a capacity of 65 bytes.
	prec := 4
	if f.precPresent && f.prec > 4 {
		prec = f.prec
		// Compute space needed for "U+" , number, " '", character, "'".
		width := 2 + prec + 2 + utf8.UTFMax + 1
		if width > cap(buf) {
			buf = make([]byte, width)
		}
	}

	// Format into buf, ending at buf[i]. Formatting numbers is easier right-to-left.
	i := len(buf)

	// For %#U we want to add a space and a quoted character at the end of the buffer.
	if f.sharp && u <= utf8.MaxRune && strconv.IsPrint(rune(u)) {
		i--
		buf[i] = '\''
		i -= utf8.RuneLen(rune(u))
		utf8.EncodeRune(buf[i:], rune(u))
		i--
		buf[i] = '\''
		i--
		buf[i] = ' '
	}
	// Format the Unicode code point u as a hexadecimal number.
	for u >= 16 {
		i--
		buf[i] = udigits[u&0xF]
		prec--
		u >>= 4
	}
	i--
	buf[i] = udigits[u]
	prec--
	// Add zeros in front of the number until requested precision is reached.
	for prec > 0 {
		i--
		buf[i] = '0'
		prec--
	}
	// Add a leading "U+".
	i--
	buf[i] = '+'
	i--
	buf[i] = 'U'

	oldZero := f.zero
	f.zero = false
	f.pad(buf[i:])
	f.zero = oldZero
}

// integer; interprets prec but not wid. Once formatted, result is sent to pad()
// and then flags are cleared.
func (f *fmt) integer(a int64, base uint64, signedness bool, digits string) {
	// precision of 0 and value of 0 means "print nothing"
	if f.precPresent && f.prec == 0 && a == 0 {
		return
	}

	negative := signedness == signed && a < 0
	if negative {
		a = -a
	}

	var buf []byte = f.intbuf[0:]
	if f.widPresent || f.precPresent || f.plus || f.space {
		width := f.wid + f.prec // Only one will be set, both are positive; this provides the maximum.
		if base == 16 && f.sharp {
			// Also adds "0x".
			width += 2
		}
		if negative || f.plus || f.space {
			width++
		}
		if width > nByte {
			// We're going to need a bigger boat.
			buf = make([]byte, width)
		}
	}

	// two ways to ask for extra leading zero digits: %.3d or %03d.
	// apparently the first cancels the second.
	prec := 0
	if f.precPresent {
		prec = f.prec
		f.zero = false
	} else if f.zero && f.widPresent && !f.minus && f.wid > 0 {
		prec = f.wid
		if negative || f.plus || f.space {
			prec-- // leave room for sign
		}
	}

	// format a into buf, ending at buf[i].  (printing is easier right-to-left.)
	// a is made into unsigned ua.  we could make things
	// marginally faster by splitting the 32-bit case out into a separate
	// block but it's not worth the duplication, so ua has 64 bits.
	i := len(buf)
	ua := uint64(a)
	// use constants for the division and modulo for more efficient code.
	// switch cases ordered by popularity.
	switch base {
	case 10:
		for ua >= 10 {
			i--
			next := ua / 10
			buf[i] = byte('0' + ua - next*10)
			ua = next
		}
	case 16:
		for ua >= 16 {
			i--
			buf[i] = digits[ua&0xF]
			ua >>= 4
		}
	case 8:
		for ua >= 8 {
			i--
			buf[i] = byte('0' + ua&7)
			ua >>= 3
		}
	case 2:
		for ua >= 2 {
			i--
			buf[i] = byte('0' + ua&1)
			ua >>= 1
		}
	default:
		panic("fmt: unknown base; can't happen")
	}
	i--
	buf[i] = digits[ua]
	for i > 0 && prec > len(buf)-i {
		i--
		buf[i] = '0'
	}

	// Various prefixes: 0x, -, etc.
	if f.sharp {
		switch base {
		case 8:
			if buf[i] != '0' {
				i--
				buf[i] = '0'
			}
		case 16:
			// Add a leading 0x or 0X.
			i--
			buf[i] = digits[16]
			i--
			buf[i] = '0'
		}
	}

	if negative {
		i--
		buf[i] = '-'
	} else if f.plus {
		i--
		buf[i] = '+'
	} else if f.space {
		i--
		buf[i] = ' '
	}

	f.pad(buf[i:])
}

// truncate truncates the string to the specified precision, if present.
func (f *fmt) truncate(s string) string {
	if f.precPresent {
		n := f.prec
		for i := range s {
			n--
			if n < 0 {
				return s[:i]
			}
		}
	}
	return s
}

// fmt_s formats a string.
func (f *fmt) fmt_s(s string) {
	s = f.truncate(s)
	f.padString(s)
}

// fmt_sbx formats a string or byte slice as a hexadecimal encoding of its bytes.
func (f *fmt) fmt_sbx(s string, b []byte, digits string) {
	length := len(b)
	if b == nil {
		// No byte slice present. Assume string s should be encoded.
		length = len(s)
	}
	// Set length to not process more bytes than the precision demands.
	if f.precPresent && f.prec < length {
		length = f.prec
	}
	// Compute width of the encoding taking into account the f.sharp and f.space flag.
	width := 2 * length
	if width > 0 {
		if f.space {
			// Each element encoded by two hexadecimals will get a leading 0x or 0X.
			if f.sharp {
				width *= 2
			}
			// Elements will be separated by a space.
			width += length - 1
		} else if f.sharp {
			// Only a leading 0x or 0X will be added for the whole string.
			width += 2
		}
	} else { // The byte slice or string that should be encoded is empty.
		if f.widPresent {
			f.writePadding(f.wid)
		}
		return
	}
	// Handle padding to the left.
	if f.widPresent && f.wid > width && !f.minus {
		f.writePadding(f.wid - width)
	}
	// Write the encoding directly into the output buffer.
	buf := *f.buf
	if f.sharp {
		// Add leading 0x or 0X.
		buf = append(buf, '0', digits[16])
	}
	var c byte
	for i := 0; i < length; i++ {
		if f.space && i > 0 {
			// Separate elements with a space.
			buf = append(buf, ' ')
			if f.sharp {
				// Add leading 0x or 0X for each element.
				buf = append(buf, '0', digits[16])
			}
		}
		if b != nil {
			c = b[i] // Take a byte from the input byte slice.
		} else {
			c = s[i] // Take a byte from the input string.
		}
		// Encode each byte as two hexadecimal digits.
		buf = append(buf, digits[c>>4], digits[c&0xF])
	}
	*f.buf = buf
	// Handle padding to the right.
	if f.widPresent && f.wid > width && f.minus {
		f.writePadding(f.wid - width)
	}
}

// fmt_sx formats a string as a hexadecimal encoding of its bytes.
func (f *fmt) fmt_sx(s, digits string) {
	f.fmt_sbx(s, nil, digits)
}

// fmt_bx formats a byte slice as a hexadecimal encoding of its bytes.
func (f *fmt) fmt_bx(b []byte, digits string) {
	f.fmt_sbx("", b, digits)
}

// fmt_q formats a string as a double-quoted, escaped Go string constant.
// If f.sharp is set a raw (backquoted) string may be returned instead
// if the string does not contain any control characters other than tab.
func (f *fmt) fmt_q(s string) {
	s = f.truncate(s)
	if f.sharp && strconv.CanBackquote(s) {
		f.padString("`" + s + "`")
		return
	}
	buf := f.intbuf[:0]
	if f.plus {
		f.pad(strconv.AppendQuoteToASCII(buf, s))
	} else {
		f.pad(strconv.AppendQuote(buf, s))
	}
}

// fmt_c formats an integer as a Unicode character.
// If the character is not valid Unicode, it will print '\ufffd'.
func (f *fmt) fmt_c(c uint64) {
	r := rune(c)
	if c > utf8.MaxRune {
		r = utf8.RuneError
	}
	buf := f.intbuf[:0]
	w := utf8.EncodeRune(buf[:utf8.UTFMax], r)
	f.pad(buf[:w])
}

// fmt_qc formats an integer as a single-quoted, escaped Go character constant.
// If the character is not valid Unicode, it will print '\ufffd'.
func (f *fmt) fmt_qc(c uint64) {
	r := rune(c)
	if c > utf8.MaxRune {
		r = utf8.RuneError
	}
	buf := f.intbuf[:0]
	if f.plus {
		f.pad(strconv.AppendQuoteRuneToASCII(buf, r))
	} else {
		f.pad(strconv.AppendQuoteRune(buf, r))
	}
}

// fmt_float formats a float64. It assumes that verb is a valid format specifier
// for strconv.AppendFloat and therefore fits into a byte.
func (f *fmt) fmt_float(v float64, size int, verb rune, prec int) {
	// Explicit precision in format specifier overrules default precision.
	if f.precPresent {
		prec = f.prec
	}
	// Format number, reserving space for leading + sign if needed.
	num := strconv.AppendFloat(f.intbuf[:1], v, byte(verb), prec, size)
	if num[1] == '-' || num[1] == '+' {
		num = num[1:]
	} else {
		num[0] = '+'
	}
	// f.space means to add a leading space instead of a "+" sign unless
	// the sign is explicitly asked for by f.plus.
	if f.space && num[0] == '+' && !f.plus {
		num[0] = ' '
	}
	// Special handling for infinities and NaN,
	// which don't look like a number so shouldn't be padded with zeros.
	if num[1] == 'I' || num[1] == 'N' {
		oldZero := f.zero
		f.zero = false
		// Remove sign before NaN if not asked for.
		if num[1] == 'N' && !f.space && !f.plus {
			num = num[1:]
		}
		f.pad(num)
		f.zero = oldZero
		return
	}
	// We want a sign if asked for and if the sign is not positive.
	if f.plus || num[0] != '+' {
		// If we're zero padding we want the sign before the leading zeros.
		// Achieve this by writing the sign out and then padding the unsigned number.
		if f.zero && f.widPresent && f.wid > len(num) {
			f.buf.WriteByte(num[0])
			f.wid--
			f.pad(num[1:])
			f.wid++
			return
		}
		f.pad(num)
		return
	}
	// No sign to show and the number is positive; just print the unsigned number.
	f.pad(num[1:])
}
