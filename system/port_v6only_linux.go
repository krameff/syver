package system

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Wire sizes and attribute numbers from linux/inet_diag.h. x/sys/unix has the
// netlink constants but none of the inet_diag ones.
const (
	inetDiagReqV2Len  = 56 // struct inet_diag_req_v2
	inetDiagMsgLen    = 72 // struct inet_diag_msg
	inetDiagSkV6Only  = 11 // INET_DIAG_SKV6ONLY: one byte, the socket's IPV6_V6ONLY
	tcpListenStateBit = 1 << 10
	nlaTypeMask       = 0x3fff // strips NLA_F_NESTED and NLA_F_NET_BYTEORDER
)

// sockDiagRecvTimeout bounds each receive, and sockDiagDeadline the whole
// dump including every receive, so a kernel that never sends NLMSG_DONE
// cannot hang a run.
const (
	sockDiagRecvTimeout = 2 * time.Second
	sockDiagDeadline    = 5 * time.Second
)

var sockDiagSeq atomic.Uint32

// wildcardV6Only returns, for every socket of proto (protoTCP or protoUDP)
// bound to [::], its port and whether it is IPv6-only, read through
// NETLINK_SOCK_DIAG. A port with several such sockets is v6onlyOff if any one
// of them is. The kernel answers for the caller's own network namespace.
func wildcardV6Only(proto uint8) (map[uint32]v6onlyState, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, fmt.Errorf("sock_diag socket: %w", err)
	}
	defer unix.Close(fd)

	seq := sockDiagSeq.Add(1)
	if err := unix.Sendto(fd, inetDiagRequest(proto, seq), 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("sock_diag request: %w", err)
	}

	return readInetDiagDump(fd, seq, time.Now().Add(sockDiagDeadline), sockDiagRecvTimeout)
}

// readInetDiagDump reads the reply to dump request seq from fd until
// NLMSG_DONE, an error, or deadline.
func readInetDiagDump(fd int, seq uint32, deadline time.Time, recvTimeout time.Duration) (map[uint32]v6onlyState, error) {
	states := make(map[uint32]v6onlyState)
	buf := make([]byte, 64*1024)
	for {
		// Each receive waits at most recvTimeout and never past deadline. A
		// zero SO_RCVTIMEO means no timeout, so the wait is at least a
		// millisecond.
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errors.New("sock_diag: no end of dump before the deadline")
		}
		wait := max(min(recvTimeout, remaining), time.Millisecond)
		tv := unix.NsecToTimeval(wait.Nanoseconds())
		if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
			return nil, fmt.Errorf("sock_diag receive timeout: %w", err)
		}
		n, from, err := unix.Recvfrom(fd, buf, unix.MSG_TRUNC)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("sock_diag: no reply within %v", wait)
		}
		if err != nil {
			return nil, fmt.Errorf("sock_diag receive: %w", err)
		}
		if n > len(buf) {
			return nil, fmt.Errorf("sock_diag: message of %d bytes truncated to %d", n, len(buf))
		}
		// Only the kernel (netlink port 0) answers a dump.
		if sa, ok := from.(*unix.SockaddrNetlink); !ok || sa.Pid != 0 {
			continue
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("sock_diag: %w", err)
		}
		for _, m := range msgs {
			if m.Header.Seq != seq {
				continue
			}
			switch m.Header.Type {
			case unix.NLMSG_DONE:
				if err := doneError(m.Data); err != nil {
					return nil, err
				}
				return states, nil
			case unix.NLMSG_ERROR:
				if len(m.Data) < 4 {
					return nil, errors.New("sock_diag: short NLMSG_ERROR")
				}
				if err := kernelErrno(m.Data); err != nil {
					return nil, err
				}
			case unix.SOCK_DIAG_BY_FAMILY:
				if err := foldInetDiag(states, m.Data, binary.NativeEndian); err != nil {
					return nil, err
				}
			}
		}
	}
}

// doneError returns the error a kernel reports in an NLMSG_DONE payload when
// it ends a dump early, such as ENOENT for a protocol with no sock_diag
// handler. A DONE with no payload, or a zero code, ends a complete dump.
func doneError(data []byte) error {
	if len(data) < 4 {
		return nil
	}
	return kernelErrno(data)
}

// kernelErrno reads the negative errno at the start of an NLMSG_ERROR or
// NLMSG_DONE payload, which must be at least 4 bytes; zero is no error.
func kernelErrno(data []byte) error {
	if code := int32(binary.NativeEndian.Uint32(data[:4])); code != 0 {
		return fmt.Errorf("sock_diag: kernel returned %w", unix.Errno(-code))
	}
	return nil
}

// inetDiagRequest builds a SOCK_DIAG_BY_FAMILY dump request for every AF_INET6
// socket of proto: listening sockets for TCP, every socket for UDP, which has
// no listen state.
func inetDiagRequest(proto uint8, seq uint32) []byte {
	b := make([]byte, unix.SizeofNlMsghdr+inetDiagReqV2Len)
	ne := binary.NativeEndian
	ne.PutUint32(b[0:4], uint32(len(b)))
	ne.PutUint16(b[4:6], unix.SOCK_DIAG_BY_FAMILY)
	ne.PutUint16(b[6:8], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	ne.PutUint32(b[8:12], seq)
	// b[12:16], the sender port, stays 0: the kernel fills it in.
	req := b[unix.SizeofNlMsghdr:]
	req[0] = unix.AF_INET6
	req[1] = proto
	// req[2] ext and req[3] pad stay 0: INET_DIAG_SKV6ONLY is sent unrequested.
	states := ^uint32(0)
	if proto == protoTCP {
		states = tcpListenStateBit
	}
	ne.PutUint32(req[4:8], states)
	// req[8:56], the inet_diag_sockid filter, stays zero for a dump.
	return b
}

// foldInetDiag decodes one inet_diag_msg and, when the socket is AF_INET6 on
// the unspecified address, merges its state into states: v6onlyOff wins over
// anything, then v6onlyUnknown, then v6onlyOn.
func foldInetDiag(states map[uint32]v6onlyState, b []byte, attrOrder binary.ByteOrder) error {
	src, port, st, err := decodeInetDiag(b, attrOrder)
	if err != nil {
		return err
	}
	if b[0] != unix.AF_INET6 || src != [16]byte{} {
		return nil
	}
	key := uint32(port)
	prev, seen := states[key]
	switch {
	case !seen, st == v6onlyOff, st == v6onlyUnknown && prev == v6onlyOn:
		states[key] = st
	}
	return nil
}

// decodeInetDiag parses one inet_diag_msg payload (the netlink message body,
// header already stripped). attrOrder is the byte order of rtattr headers:
// binary.NativeEndian in production; tests also pass BigEndian. The port is
// always big-endian, by protocol.
func decodeInetDiag(b []byte, attrOrder binary.ByteOrder) (src [16]byte, port uint16, st v6onlyState, err error) {
	if len(b) < inetDiagMsgLen {
		return src, 0, v6onlyUnknown, fmt.Errorf("sock_diag: inet_diag_msg of %d bytes, want at least %d", len(b), inetDiagMsgLen)
	}
	// inet_diag_msg: family, state, timer, retrans (one byte each), then
	// inet_diag_sockid: sport, dport (__be16), src, dst (__be32[4]), ...
	port = binary.BigEndian.Uint16(b[4:6])
	copy(src[:], b[8:24])

	st = v6onlyUnknown
	attrs := b[inetDiagMsgLen:]
	for len(attrs) >= 4 {
		l := int(attrOrder.Uint16(attrs[0:2]))
		typ := attrOrder.Uint16(attrs[2:4]) & nlaTypeMask
		if l < 4 || l > len(attrs) {
			break
		}
		if typ == inetDiagSkV6Only && l >= 5 {
			switch attrs[4] {
			case 0:
				st = v6onlyOff
			case 1:
				st = v6onlyOn
			}
		}
		next := (l + 3) &^ 3 // rtattrs are 4-byte aligned
		if next > len(attrs) {
			break
		}
		attrs = attrs[next:]
	}
	return src, port, st, nil
}
