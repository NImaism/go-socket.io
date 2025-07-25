package polling

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NImaism/go-socket.io/engineio/frame"
	"github.com/NImaism/go-socket.io/engineio/packet"
	"github.com/NImaism/go-socket.io/engineio/transport"
)

var tests = []struct {
	ft   frame.Type
	pt   packet.Type
	data []byte
}{
	{frame.String, packet.OPEN, []byte{}},
	{frame.String, packet.MESSAGE, []byte("hello")},
	{frame.Binary, packet.MESSAGE, []byte{1, 2, 3, 4}},
}

func TestPollingBinary(t *testing.T) {
	should := assert.New(t)
	must := require.New(t)

	var scValue atomic.Value

	pollingTransport := Default
	should.Equal("polling", pollingTransport.Name())

	conn := make(chan transport.Conn, 1)
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Eio-Test", "server")
		c := scValue.Load()
		if c == nil {
			co, err := pollingTransport.Accept(w, r)
			should.Nil(err)
			scValue.Store(co)
			c = co
			conn <- co
		}
		c.(http.Handler).ServeHTTP(w, r)
	}

	httpSvr := httptest.NewServer(http.HandlerFunc(handler))
	defer httpSvr.Close()

	u, err := url.Parse(httpSvr.URL)
	should.Nil(err)

	dialU := *u

	header := make(http.Header)
	header.Set("X-Eio-Test", "client")
	cc, err := pollingTransport.Dial(&dialU, header)
	should.Nil(err)

	cc.(*clientConn).Resume()
	defer func() {
		must.NoError(cc.Close())
	}()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		for _, test := range tests {
			ft, pt, r, err := cc.NextReader()
			should.Nil(err)

			should.Equal(test.ft, ft)
			should.Equal(test.pt, pt)
			b, err := ioutil.ReadAll(r)
			should.Nil(err)
			should.Equal(test.data, b)
			err = r.Close()
			should.Nil(err)

			w, err := cc.NextWriter(ft, pt)
			should.Nil(err)
			_, err = w.Write(b)
			should.Nil(err)
			err = w.Close()
			should.Nil(err)
		}
	}()

	sc := <-conn
	defer func() {
		must.NoError(sc.Close())
	}()

	for _, test := range tests {
		w, err := sc.NextWriter(test.ft, test.pt)
		should.Nil(err)
		_, err = w.Write(test.data)
		should.Nil(err)
		err = w.Close()
		should.Nil(err)

		ft, pt, r, err := sc.NextReader()
		should.Nil(err)
		should.Equal(test.ft, ft)
		should.Equal(test.pt, pt)
		b, err := ioutil.ReadAll(r)
		should.Nil(err)
		err = r.Close()
		should.Nil(err)
		should.Equal(test.data, b)
	}

	wg.Wait()

	should.Equal(sc.LocalAddr(), cc.RemoteAddr())
	should.Empty(cc.LocalAddr().String())
	should.NotEmpty(sc.RemoteAddr().String())
	should.Equal("server", cc.RemoteHeader().Get("X-Eio-Test"))
	should.Equal("client", sc.RemoteHeader().Get("X-Eio-Test"))
}

func TestPollingString(t *testing.T) {
	should := assert.New(t)
	must := require.New(t)

	var scValue atomic.Value

	pollingTransport := Default
	conn := make(chan transport.Conn, 1)
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Eio-Test", "server")
		c := scValue.Load()
		if c == nil {
			co, err := pollingTransport.Accept(w, r)
			should.Nil(err)
			scValue.Store(co)
			c = co
			conn <- co
		}
		c.(http.Handler).ServeHTTP(w, r)
	}
	httpSvr := httptest.NewServer(http.HandlerFunc(handler))
	defer httpSvr.Close()

	u, err := url.Parse(httpSvr.URL)
	should.Nil(err)

	query := u.Query()
	query.Set("b64", "1")
	u.RawQuery = query.Encode()

	dialU := *u
	header := make(http.Header)
	header.Set("X-Eio-Test", "client")
	cc, err := pollingTransport.Dial(&dialU, header)
	should.Nil(err)

	cc.(*clientConn).Resume()
	defer func() {
		must.NoError(cc.Close())
	}()

	sc := <-conn
	defer func() {
		must.NoError(sc.Close())
	}()

	should.Equal(sc.LocalAddr(), cc.RemoteAddr())
	should.Equal("tcp", sc.LocalAddr().Network())
	should.Empty(cc.LocalAddr().String())
	should.NotEmpty(sc.RemoteAddr().String())

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		for _, test := range tests {
			ft, pt, r, err := cc.NextReader()
			should.Nil(err)

			should.Equal(test.ft, ft)
			should.Equal(test.pt, pt)
			b, err := ioutil.ReadAll(r)
			should.Nil(err)
			err = r.Close()
			should.Nil(err)
			should.Equal(test.data, b)

			w, err := cc.NextWriter(ft, pt)
			should.Nil(err)
			_, err = w.Write(b)
			should.Nil(err)
			err = w.Close()
			should.Nil(err)
		}
	}()

	for _, test := range tests {
		w, err := sc.NextWriter(test.ft, test.pt)
		should.Nil(err)

		_, err = w.Write(test.data)
		should.Nil(err)
		should.Nil(w.Close())

		ft, pt, r, err := sc.NextReader()
		should.Nil(err)
		should.Equal(test.ft, ft)
		should.Equal(test.pt, pt)

		b, err := ioutil.ReadAll(r)
		should.Nil(err)
		should.Nil(r.Close())
		should.Equal(test.data, b)
	}

	wg.Wait()

	should.Equal("server", cc.RemoteHeader().Get("X-Eio-Test"))
	should.Equal("client", sc.RemoteHeader().Get("X-Eio-Test"))
}
