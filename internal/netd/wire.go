//go:build linux

package netd

import (
	"errors"
	"io"
	"net"
	"os"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

const (
	ProtocolMajor = 1
	ProtocolMinor = 0
	maxPacket     = 4096
	maxRights     = 4
)

var errProtocol = errors.New("netd protocol violation")

func readPacket(c *net.UnixConn) ([]byte, []*os.File, error) {
	buf := make([]byte, maxPacket)
	oob := make([]byte, unix.CmsgSpace(4*maxRights))
	n, oobn, flags, _, err := c.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, nil, err
	}
	files, rerr := rights(oob[:oobn])
	if rerr != nil || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 {
		closeAll(files)
		return nil, nil, errProtocol
	}
	if n == 0 && len(files) == 0 {
		return nil, nil, io.EOF
	}
	return buf[:n], files, nil
}

func rights(oob []byte) ([]*os.File, error) {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}
	var files []*os.File
	for _, m := range msgs {
		fds, err := unix.ParseUnixRights(&m)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			_ = unix.SetNonblock(fd, true)
			files = append(files, os.NewFile(uintptr(fd), "received"))
		}
	}
	return files, nil
}

func closeAll(files []*os.File) {
	for _, f := range files {
		f.Close()
	}
}

func writePacket(c *net.UnixConn, m proto.Message, f *os.File) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > maxPacket {
		return errProtocol
	}
	var oob []byte
	if f != nil {
		raw, err := f.SyscallConn()
		if err != nil {
			return err
		}
		if cerr := raw.Control(func(fd uintptr) { oob = unix.UnixRights(int(fd)) }); cerr != nil {
			return cerr
		}
	}
	_, _, err = c.WriteMsgUnix(b, oob, nil)
	return err
}
