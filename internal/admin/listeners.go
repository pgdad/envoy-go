package admin

import (
	"bytes"
	"net/http"
	"sort"
)

// handleListeners implements /listeners per SPEC §5.4 + §11.3 + ADR-0087.
// Body is `text/plain; charset=UTF-8`. One line per listener:
// `<name>::<bind_addr>` where bind_addr is the listener's resolved
// `host:port` form (square-bracket-wrapped for IPv6 hosts as
// net.Listener.Addr().String() emits). Alphabetical-by-name; trailing newline
// after each line.
//
// nil-lm path: when admin.New is called with lm=nil (test convention per
// ADR-0085's nil-tolerated constructor), the handler emits an empty body
// + 200 OK rather than panicking. Production call sites always thread a
// non-nil listener.Manager (see cmd/envoy-go/main.go, Task 10).
//
// The s.lm.Listeners() snapshot returns one Info per bound listener (empty
// before Start or after a Start that errored out). The sort here is
// defensive — Listeners() iterates m.runtimes in declaration order, which
// is NOT guaranteed alphabetical — so the §11.3 alphabetical-by-name
// ordering is enforced here at scrape time.
func (s *Server) handleListeners(w http.ResponseWriter, _ *http.Request) {
	var buf bytes.Buffer
	if s.lm != nil {
		infos := s.lm.Listeners()
		sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
		for _, li := range infos {
			buf.WriteString(li.Name)
			buf.WriteString("::")
			buf.WriteString(li.Addr)
			buf.WriteByte('\n')
		}
	}
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
