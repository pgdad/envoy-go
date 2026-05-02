package tls_inspector

import "encoding/binary"

// parseClientHello extracts the SNI server_name + ALPN application_protocols
// from a TLS ClientHello byte buffer. Returns ok=false on any malformed
// input (truncated, wrong record type, length-prefix mismatch). Pure
// function; no I/O. Adapted from crypto/tls/handshake_messages.go:unmarshal
// for the ClientHello case, narrowed to the two extensions of interest.
func parseClientHello(buf []byte) (sni string, alpns []string, ok bool) {
	// TLS record header: 5 bytes.
	if len(buf) < 5 {
		return "", nil, false
	}
	if buf[0] != 0x16 {
		return "", nil, false
	} // not a Handshake record
	// buf[1:3] = legacy_version (TLS 1.0–1.2 marker; ClientHello is allowed
	// to have any value here per RFC 8446 §4.1.2).
	recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if 5+recordLen > len(buf) {
		return "", nil, false
	} // truncated record
	body := buf[5 : 5+recordLen]
	// Handshake header: 4 bytes.
	if len(body) < 4 {
		return "", nil, false
	}
	if body[0] != 0x01 {
		return "", nil, false
	} // not a ClientHello
	hsLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if 4+hsLen > len(body) {
		return "", nil, false
	}
	ch := body[4 : 4+hsLen]
	// ClientHello body: legacy_version (2) + random (32) + session_id_length (1) + ...
	off := 0
	if off+2+32+1 > len(ch) {
		return "", nil, false
	}
	off += 2 + 32 // skip legacy_version + random
	sidLen := int(ch[off])
	off++
	if off+sidLen+2 > len(ch) {
		return "", nil, false
	}
	off += sidLen
	csLen := int(binary.BigEndian.Uint16(ch[off : off+2]))
	off += 2
	if off+csLen+1 > len(ch) {
		return "", nil, false
	}
	off += csLen
	cmLen := int(ch[off])
	off++
	if off+cmLen+2 > len(ch) {
		return "", nil, false
	}
	off += cmLen
	if off+2 > len(ch) {
		return "", nil, true
	} // no extensions block — valid
	extLen := int(binary.BigEndian.Uint16(ch[off : off+2]))
	off += 2
	if off+extLen > len(ch) {
		return "", nil, false
	}
	exts := ch[off : off+extLen]
	// Iterate extensions.
	for len(exts) >= 4 {
		typ := binary.BigEndian.Uint16(exts[:2])
		ln := int(binary.BigEndian.Uint16(exts[2:4]))
		if 4+ln > len(exts) {
			return "", nil, false
		}
		body := exts[4 : 4+ln]
		switch typ {
		case 0x0000: // server_name
			if name, ok := parseServerName(body); ok {
				sni = name
			}
		case 0x0010: // application_layer_protocol_negotiation
			if al, ok := parseALPN(body); ok {
				alpns = al
			}
		}
		exts = exts[4+ln:]
	}
	return sni, alpns, true
}

// parseServerName walks the ServerNameList per RFC 6066 §3. Returns the
// first host_name (NameType 0) — Envoy convention.
func parseServerName(buf []byte) (string, bool) {
	if len(buf) < 2 {
		return "", false
	}
	listLen := int(binary.BigEndian.Uint16(buf[:2]))
	if 2+listLen > len(buf) {
		return "", false
	}
	list := buf[2 : 2+listLen]
	for len(list) >= 3 {
		nameType := list[0]
		nameLen := int(binary.BigEndian.Uint16(list[1:3]))
		if 3+nameLen > len(list) {
			return "", false
		}
		if nameType == 0x00 { // host_name
			return string(list[3 : 3+nameLen]), true
		}
		list = list[3+nameLen:]
	}
	return "", false
}

// parseALPN walks the ProtocolNameList per RFC 7301 §3.1. Returns every
// protocol_name in declaration order.
func parseALPN(buf []byte) ([]string, bool) {
	if len(buf) < 2 {
		return nil, false
	}
	listLen := int(binary.BigEndian.Uint16(buf[:2]))
	if 2+listLen > len(buf) {
		return nil, false
	}
	list := buf[2 : 2+listLen]
	var out []string
	for len(list) >= 1 {
		nameLen := int(list[0])
		if 1+nameLen > len(list) {
			return nil, false
		}
		out = append(out, string(list[1:1+nameLen]))
		list = list[1+nameLen:]
	}
	return out, true
}
