package netd

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"os"

	"google.golang.org/protobuf/proto"
)

const maxFrame = 0xffff

func readPacket(c net.Conn) ([]byte, []*os.File, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return nil, nil, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n > maxPacket {
		return nil, nil, errProtocol
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(c, b); err != nil {
		return nil, nil, err
	}
	return b, nil, nil
}

func writePacket(c net.Conn, m proto.Message, _ *os.File) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > maxPacket {
		return errProtocol
	}
	return writeFrame(c, b)
}

func writeFrame(w io.Writer, b []byte) error {
	out := make([]byte, 2+len(b))
	binary.BigEndian.PutUint16(out, uint16(len(b)))
	copy(out[2:], b)
	_, err := w.Write(out)
	return err
}

type framedPackets struct {
	net.Conn
	r *bufio.Reader
}

func newFramedPackets(c net.Conn) *framedPackets {
	return &framedPackets{Conn: c, r: bufio.NewReaderSize(c, 4*maxFrame)}
}

func (f *framedPackets) Read(p []byte) (int, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(f.r, hdr[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n > len(p) {
		return 0, errProtocol
	}
	if _, err := io.ReadFull(f.r, p[:n]); err != nil {
		return 0, err
	}
	return n, nil
}

func (f *framedPackets) Write(p []byte) (int, error) {
	if len(p) > maxFrame {
		return 0, errProtocol
	}
	if err := writeFrame(f.Conn, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
