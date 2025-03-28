// Copyright 2019 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build (linux && amd64) || (linux && arm64)
// +build linux,amd64 linux,arm64

package fdbased

import (
	"encoding/binary"
	"fmt"

	"github.com/talismancer/gvisor-ligolo/pkg/buffer"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/header"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/rawfile"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/link/stopfd"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/stack"
	"golang.org/x/sys/unix"
)

const (
	tPacketAlignment = uintptr(16)
	tpStatusKernel   = 0
	tpStatusUser     = 1
	tpStatusCopy     = 2
	tpStatusLosing   = 4
)

// We overallocate the frame size to accommodate space for the
// TPacketHdr+RawSockAddrLinkLayer+MAC header and any padding.
//
// Memory allocated for the ring buffer: tpBlockSize * tpBlockNR = 2 MiB
//
// NOTE:
//
//	Frames need to be aligned at 16 byte boundaries.
//	BlockSize needs to be page aligned.
//
//	For details see PACKET_MMAP setting constraints in
//	https://www.kernel.org/doc/Documentation/networking/packet_mmap.txt
const (
	tpFrameSize = 65536 + 128
	tpBlockSize = tpFrameSize * 32
	tpBlockNR   = 1
	tpFrameNR   = (tpBlockSize * tpBlockNR) / tpFrameSize
)

// tPacketAlign aligns the pointer v at a tPacketAlignment boundary. Direct
// translation of the TPACKET_ALIGN macro in <linux/if_packet.h>.
func tPacketAlign(v uintptr) uintptr {
	return (v + tPacketAlignment - 1) & uintptr(^(tPacketAlignment - 1))
}

// tPacketReq is the tpacket_req structure as described in
// https://www.kernel.org/doc/Documentation/networking/packet_mmap.txt
type tPacketReq struct {
	tpBlockSize uint32
	tpBlockNR   uint32
	tpFrameSize uint32
	tpFrameNR   uint32
}

// tPacketHdr is tpacket_hdr structure as described in <linux/if_packet.h>
type tPacketHdr []byte

const (
	tpStatusOffset  = 0
	tpLenOffset     = 8
	tpSnapLenOffset = 12
	tpMacOffset     = 16
	tpNetOffset     = 18
	tpSecOffset     = 20
	tpUSecOffset    = 24
)

func (t tPacketHdr) tpLen() uint32 {
	return binary.LittleEndian.Uint32(t[tpLenOffset:])
}

func (t tPacketHdr) tpSnapLen() uint32 {
	return binary.LittleEndian.Uint32(t[tpSnapLenOffset:])
}

func (t tPacketHdr) tpMac() uint16 {
	return binary.LittleEndian.Uint16(t[tpMacOffset:])
}

func (t tPacketHdr) tpNet() uint16 {
	return binary.LittleEndian.Uint16(t[tpNetOffset:])
}

func (t tPacketHdr) tpSec() uint32 {
	return binary.LittleEndian.Uint32(t[tpSecOffset:])
}

func (t tPacketHdr) tpUSec() uint32 {
	return binary.LittleEndian.Uint32(t[tpUSecOffset:])
}

func (t tPacketHdr) Payload() []byte {
	return t[uint32(t.tpMac()) : uint32(t.tpMac())+t.tpSnapLen()]
}

// packetMMapDispatcher uses PACKET_RX_RING's to read/dispatch inbound packets.
// See: mmap_amd64_unsafe.go for implementation details.
type packetMMapDispatcher struct {
	stopfd.StopFD
	// fd is the file descriptor used to send and receive packets.
	fd int

	// e is the endpoint this dispatcher is attached to.
	e *endpoint

	// ringBuffer is only used when PacketMMap dispatcher is used and points
	// to the start of the mmapped PACKET_RX_RING buffer.
	ringBuffer []byte

	// ringOffset is the current offset into the ring buffer where the next
	// inbound packet will be placed by the kernel.
	ringOffset int
}

func (*packetMMapDispatcher) release() {}

func (d *packetMMapDispatcher) readMMappedPacket() (*buffer.View, bool, tcpip.Error) {
	hdr := tPacketHdr(d.ringBuffer[d.ringOffset*tpFrameSize:])
	for hdr.tpStatus()&tpStatusUser == 0 {
		stopped, errno := rawfile.BlockingPollUntilStopped(d.EFD, d.fd, unix.POLLIN|unix.POLLERR)
		if errno != 0 {
			if errno == unix.EINTR {
				continue
			}
			return nil, stopped, rawfile.TranslateErrno(errno)
		}
		if stopped {
			return nil, true, nil
		}
		if hdr.tpStatus()&tpStatusCopy != 0 {
			// This frame is truncated so skip it after flipping the
			// buffer to the kernel.
			hdr.setTPStatus(tpStatusKernel)
			d.ringOffset = (d.ringOffset + 1) % tpFrameNR
			hdr = (tPacketHdr)(d.ringBuffer[d.ringOffset*tpFrameSize:])
			continue
		}
	}

	// Copy out the packet from the mmapped frame to a locally owned buffer.
	pkt := buffer.NewView(int(hdr.tpSnapLen()))
	pkt.Write(hdr.Payload())
	// Release packet to kernel.
	hdr.setTPStatus(tpStatusKernel)
	d.ringOffset = (d.ringOffset + 1) % tpFrameNR
	return pkt, false, nil
}

// dispatch reads packets from an mmaped ring buffer and dispatches them to the
// network stack.
func (d *packetMMapDispatcher) dispatch() (bool, tcpip.Error) {
	pkt, stopped, err := d.readMMappedPacket()
	if err != nil || stopped {
		return false, err
	}
	var p tcpip.NetworkProtocolNumber
	if d.e.hdrSize > 0 {
		p = header.Ethernet(pkt.AsSlice()).Type()
	} else {
		// We don't get any indication of what the packet is, so try to guess
		// if it's an IPv4 or IPv6 packet.
		switch header.IPVersion(pkt.AsSlice()) {
		case header.IPv4Version:
			p = header.IPv4ProtocolNumber
		case header.IPv6Version:
			p = header.IPv6ProtocolNumber
		default:
			return true, nil
		}
	}

	pbuf := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithView(pkt),
	})
	defer pbuf.DecRef()
	if d.e.hdrSize > 0 {
		if _, ok := pbuf.LinkHeader().Consume(d.e.hdrSize); !ok {
			panic(fmt.Sprintf("LinkHeader().Consume(%d) must succeed", d.e.hdrSize))
		}
	}
	d.e.mu.RLock()
	dsp := d.e.dispatcher
	d.e.mu.RUnlock()
	dsp.DeliverNetworkPacket(p, pbuf)
	return true, nil
}
