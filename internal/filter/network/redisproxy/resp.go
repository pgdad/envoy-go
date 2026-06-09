package redisproxy

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Overflow guards (checked BEFORE any allocation; the upstream proto_max_bulk_size
// analog). A declared length beyond these → a decode error, never an allocation.
const (
	maxBulkLen  = 512 * 1024 * 1024 // 512 MiB
	maxArrayLen = 1024 * 1024       // 1 Mi elements
)

// errProtocol is the catch-all RESP framing error. Wording is internal-only
// (never byte-compared) — the differential proof is the downstream_cx_protocol_
// error COUNT (32.2), not the message.
var errProtocol = errors.New("redis_proxy: resp: protocol error")

// unexpected maps a mid-frame io.EOF to io.ErrUnexpectedEOF (a transport/protocol
// error — the connection ended INSIDE a frame). A frame-boundary io.EOF is
// returned verbatim by decodeRequest's first read (clean connection end).
func unexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// readLine reads through the next '\n', appends the raw bytes (incl. the
// terminator) to raw, and returns the line WITHOUT the trailing "\r\n" (or bare
// "\n"). Blocks until a full line arrives (partial-frame reassembly).
func readLine(r *bufio.Reader, raw *bytes.Buffer) (string, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return "", err
	}
	raw.Write(line)
	n := len(line) - 1 // drop '\n'
	if n > 0 && line[n-1] == '\r' {
		n-- // drop '\r'
	}
	return string(line[:n]), nil
}

// readBulk reads one RESP bulk string "$<len>\r\n<len bytes>\r\n", appends the
// raw bytes to raw, and returns the payload. Used by the request array path.
func readBulk(r *bufio.Reader, raw *bytes.Buffer) ([]byte, error) {
	header, err := readLine(r, raw)
	if err != nil {
		return nil, unexpected(err)
	}
	if len(header) == 0 || header[0] != '$' {
		return nil, errProtocol
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil || n < 0 || n > maxBulkLen {
		return nil, errProtocol
	}
	body := make([]byte, n+2) // payload + trailing "\r\n"
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, unexpected(err)
	}
	raw.Write(body)
	if body[n] != '\r' || body[n+1] != '\n' {
		return nil, errProtocol
	}
	return body[:n], nil
}

// Local-reply byte-stable constants (the +PONG / -ERR wording from parent
// §11.5/§11.6). DO NOT CHANGE — these bytes are asserted byte-identical
// cross-side (the byte-equivalence prong, §8.1.1).
var (
	respPong           = []byte("+PONG\r\n")
	respAuthNoPassword = []byte("-ERR Client sent AUTH, but no password is set\r\n")
)

// decodeReply reads ONE complete reply frame (any value type incl. null sentinels
// + nested arrays) from r and returns its RAW bytes (forwarded VERBATIM
// downstream — §8.1.1). It parses only enough to find the frame boundary; it does
// not re-encode. Blocks on partial frames; never panics on arbitrary bytes.
func decodeReply(r *bufio.Reader) (raw []byte, err error) {
	var buf bytes.Buffer
	if err := decodeReplyInto(r, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeReplyInto reads one reply frame, appending consumed bytes to raw. Arrays
// recurse (nested-array support).
func decodeReplyInto(r *bufio.Reader, raw *bytes.Buffer) error {
	t, err := r.ReadByte()
	if err != nil {
		return err
	}
	raw.WriteByte(t)
	switch t {
	case '+', '-': // simple string / error: one line of arbitrary text (no content validation)
		_, err := readLine(r, raw)
		return unexpected(err)
	case ':': // integer: one line whose body MUST be numeric. The integer body is
		// validated here (non-numeric → errProtocol) — this is the one content-level
		// check on the reply path, required because RESP integers are numeric by
		// spec and a non-numeric ":x" frame is a protocol error, not a value.
		line, err := readLine(r, raw)
		if err != nil {
			return unexpected(err)
		}
		if _, err := strconv.Atoi(line); err != nil {
			return errProtocol
		}
		return nil
	case '$': // bulk: "$<len>\r\n<bytes>\r\n" OR "$-1\r\n" (null)
		header, err := readLine(r, raw)
		if err != nil {
			return unexpected(err)
		}
		n, err := strconv.Atoi(header)
		if err != nil {
			return errProtocol
		}
		if n == -1 {
			return nil // null bulk
		}
		if n < 0 || n > maxBulkLen {
			return errProtocol
		}
		body := make([]byte, n+2)
		if _, err := io.ReadFull(r, body); err != nil {
			return unexpected(err)
		}
		raw.Write(body)
		if body[n] != '\r' || body[n+1] != '\n' {
			return errProtocol
		}
		return nil
	case '*': // array: "*<n>\r\n" + n elements OR "*-1\r\n" (null)
		header, err := readLine(r, raw)
		if err != nil {
			return unexpected(err)
		}
		n, err := strconv.Atoi(header)
		if err != nil {
			return errProtocol
		}
		if n == -1 {
			return nil // null array
		}
		if n < 0 || n > maxArrayLen {
			return errProtocol
		}
		for i := 0; i < n; i++ {
			if err := decodeReplyInto(r, raw); err != nil {
				return err
			}
		}
		return nil
	default:
		return errProtocol // unknown type byte
	}
}

// decodeRequest reads ONE request frame (inline OR array-of-bulk-strings) from r
// and returns the UPPERCASED command name (for dispatch) + the RAW frame bytes
// (forwarded VERBATIM upstream when proxied). Blocks on partial frames. A
// frame-boundary io.EOF is returned verbatim (clean connection end).
func decodeRequest(r *bufio.Reader) (cmd string, raw []byte, err error) {
	p, err := r.Peek(1)
	if err != nil {
		return "", nil, err // io.EOF here = clean end between frames
	}
	var buf bytes.Buffer
	if p[0] != '*' {
		// Reject RESP reply-type first bytes: '+' (simple string), '-' (error),
		// ':' (integer), '$' (bulk string), '@' (attribute) — these are server-to-
		// client bytes and are never valid at the start of a client request.
		switch p[0] {
		case '+', '-', ':', '$', '@':
			return "", nil, errProtocol
		}
		// inline command: a space-separated token line.
		line, err := readLine(r, &buf)
		if err != nil {
			return "", nil, unexpected(err)
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return "", nil, errProtocol
		}
		return strings.ToUpper(fields[0]), buf.Bytes(), nil
	}
	// array-of-bulk: "*<n>\r\n" then n × "$<len>\r\n<bytes>\r\n".
	header, err := readLine(r, &buf)
	if err != nil {
		return "", nil, unexpected(err)
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil || n <= 0 || n > maxArrayLen {
		return "", nil, errProtocol
	}
	for i := 0; i < n; i++ {
		bulk, err := readBulk(r, &buf)
		if err != nil {
			return "", nil, err
		}
		if i == 0 {
			cmd = strings.ToUpper(string(bulk))
		}
	}
	return cmd, buf.Bytes(), nil
}
