package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"reflect"

	"github.com/NImaism/go-socket.io/engineio/session"
	"github.com/NImaism/go-socket.io/logger"
)

type FrameWriter interface {
	NextWriter(ft session.FrameType) (io.WriteCloser, error)
}

type Encoder struct {
	w FrameWriter
}

func NewEncoder(w FrameWriter) *Encoder {
	return &Encoder{
		w: w,
	}
}

func (e *Encoder) Encode(h Header, args ...interface{}) (err error) {
	var w io.WriteCloser
	w, err = e.w.NextWriter(session.TEXT)
	if err != nil {
		logger.Error("next writer session text:", err)

		return
	}

	var buffers [][]byte
	buffers, err = e.writePacket(w, h, args)
	if err != nil {
		logger.Error("write packet header with args:", err)

		return
	}

	for _, b := range buffers {
		w, err = e.w.NextWriter(session.BINARY)
		if err != nil {
			logger.Error("next writer session binary:", err)

			return
		}

		err = e.writeBuffer(w, b)
		if err != nil {
			logger.Error("write packet buffer:", err)

			return
		}
	}

	return
}

type byteWriter interface {
	io.Writer
	WriteByte(byte) error
}

type flusher interface {
	Flush() error
}

func (e *Encoder) writePacket(w io.WriteCloser, h Header, args []interface{}) ([][]byte, error) {
	defer func() {
		if err := w.Close(); err != nil {
			logger.Error("close writer:", err)
		}
	}()

	bw, ok := w.(byteWriter)
	if !ok {
		bw = bufio.NewWriter(w)
	}

	max := uint64(0)
	buffers, err := e.attachBuffer(reflect.ValueOf(args), &max)
	if err != nil {
		return nil, err
	}

	if len(buffers) > 0 && (h.Type == Event || h.Type == Ack) {
		h.Type += 3
	}

	if err := bw.WriteByte(byte(h.Type + '0')); err != nil {
		return nil, err
	}

	if h.Type == binaryAck || h.Type == binaryEvent {
		if err := e.writeUint64(bw, max); err != nil {
			return nil, err
		}
		if err := bw.WriteByte('-'); err != nil {
			return nil, err
		}
	}

	if h.Namespace != "" {
		if _, err := bw.Write([]byte(h.Namespace)); err != nil {
			return nil, err
		}
		if h.ID != 0 || args != nil {
			if err := bw.WriteByte(','); err != nil {
				return nil, err
			}
		}
	}

	if h.NeedAck {
		if err := e.writeUint64(bw, h.ID); err != nil {
			return nil, err
		}
	}

	if len(args) > 0 {
		if err := json.NewEncoder(bw).Encode(args[0]); err != nil {
			return nil, err
		}
	}

	if f, ok := bw.(flusher); ok {
		if err := f.Flush(); err != nil {
			return nil, err
		}
	}

	return buffers, nil
}

func (e *Encoder) writeUint64(w byteWriter, i uint64) error {
	base := uint64(1)
	for i/base >= 10 {
		base *= 10
	}
	for base > 0 {
		p := i / base
		if err := w.WriteByte(byte(p) + '0'); err != nil {
			return err
		}
		i -= p * base
		base /= 10
	}
	return nil
}

func (e *Encoder) attachBuffer(v reflect.Value, index *uint64) ([][]byte, error) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	var ret [][]byte
	switch v.Kind() {
	case reflect.Struct:
		if v.Type().Name() == bufferTypeName {
			if !v.CanAddr() {
				return nil, errFailedBufferAddress
			}
			buffer := v.Addr().Interface().(*Buffer)
			buffer.num = *index
			buffer.isBinary = true
			ret = append(ret, buffer.Data)
			*index++
		} else {
			for i := 0; i < v.NumField(); i++ {
				b, err := e.attachBuffer(v.Field(i), index)
				if err != nil {
					return nil, err
				}
				ret = append(ret, b...)
			}
		}

	case reflect.Array, reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			b, err := e.attachBuffer(v.Index(i), index)
			if err != nil {
				return nil, err
			}

			ret = append(ret, b...)
		}

	case reflect.Map:
		for _, key := range v.MapKeys() {
			b, err := e.attachBuffer(v.MapIndex(key), index)
			if err != nil {
				return nil, err
			}

			ret = append(ret, b...)
		}
	}

	return ret, nil
}

func (e *Encoder) writeBuffer(w io.WriteCloser, buffer []byte) error {
	defer func() {
		if closeErr := w.Close(); closeErr != nil {
			logger.Error("close writer:", closeErr)
		}
	}()

	_, err := w.Write(buffer)
	return err
}
