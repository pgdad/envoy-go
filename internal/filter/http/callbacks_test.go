package http

import (
	"net/http"
	"testing"

	"google.golang.org/protobuf/proto"
)

type fakeDecoderCB struct {
	continueCalls int
	localReplies  int
	routeCfgCalls int
}

func (c *fakeDecoderCB) ContinueDecoding()                       { c.continueCalls++ }
func (c *fakeDecoderCB) SendLocalReply(int, string, OrderedHeaders) { c.localReplies++ }
func (c *fakeDecoderCB) RequestRouteConfig() proto.Message       { c.routeCfgCalls++; return nil }
func (c *fakeDecoderCB) EncodeHeaders(http.Header, bool)                            {}
func (c *fakeDecoderCB) EncodeData([]byte, bool)                                    {}
func (c *fakeDecoderCB) EncodeTrailers(http.Header)                                 {}

func TestDecoderFilterCallbacks_Compile(t *testing.T) {
	var _ DecoderFilterCallbacks = (*fakeDecoderCB)(nil)
}

type fakeEncoderCB struct {
	continueCalls int
}

func (c *fakeEncoderCB) ContinueEncoding()                {}
func (c *fakeEncoderCB) EncodeHeaders(http.Header, bool)  {}
func (c *fakeEncoderCB) EncodeData([]byte, bool)          {}
func (c *fakeEncoderCB) EncodeTrailers(http.Header)       {}

func TestEncoderFilterCallbacks_Compile(t *testing.T) {
	var _ EncoderFilterCallbacks = (*fakeEncoderCB)(nil)
}
