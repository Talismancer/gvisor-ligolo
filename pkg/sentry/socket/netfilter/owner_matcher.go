// Copyright 2020 The gVisor Authors.
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

package netfilter

import (
	"fmt"

	"github.com/talismancer/gvisor-ligolo/pkg/abi/linux"
	"github.com/talismancer/gvisor-ligolo/pkg/marshal"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel"
	"github.com/talismancer/gvisor-ligolo/pkg/sentry/kernel/auth"
	"github.com/talismancer/gvisor-ligolo/pkg/tcpip/stack"
)

const matcherNameOwner = "owner"

func init() {
	registerMatchMaker(ownerMarshaler{})
}

// ownerMarshaler implements matchMaker for owner matching.
type ownerMarshaler struct{}

// name implements matchMaker.name.
func (ownerMarshaler) name() string {
	return matcherNameOwner
}

// marshal implements matchMaker.marshal.
func (ownerMarshaler) marshal(mr matcher) []byte {
	matcher := mr.(*OwnerMatcher)
	iptOwnerInfo := linux.IPTOwnerInfo{
		UID: uint32(matcher.uid),
		GID: uint32(matcher.gid),
	}

	// Support for UID and GID match.
	if matcher.matchUID {
		iptOwnerInfo.Match = linux.XT_OWNER_UID
		if matcher.invertUID {
			iptOwnerInfo.Invert = linux.XT_OWNER_UID
		}
	}
	if matcher.matchGID {
		iptOwnerInfo.Match |= linux.XT_OWNER_GID
		if matcher.invertGID {
			iptOwnerInfo.Invert |= linux.XT_OWNER_GID
		}
	}

	buf := marshal.Marshal(&iptOwnerInfo)
	return marshalEntryMatch(matcherNameOwner, buf)
}

// unmarshal implements matchMaker.unmarshal.
func (ownerMarshaler) unmarshal(task *kernel.Task, buf []byte, filter stack.IPHeaderFilter) (stack.Matcher, error) {
	if len(buf) < linux.SizeOfIPTOwnerInfo {
		return nil, fmt.Errorf("buf has insufficient size for owner match: %d", len(buf))
	}

	// For alignment reasons, the match's total size may
	// exceed what's strictly necessary to hold matchData.
	var matchData linux.IPTOwnerInfo
	matchData.UnmarshalUnsafe(buf)
	nflog("parsed IPTOwnerInfo: %+v", matchData)

	var owner OwnerMatcher
	creds := task.Credentials()
	owner.uid = creds.UserNamespace.MapToKUID(auth.UID(matchData.UID))
	owner.gid = creds.UserNamespace.MapToKGID(auth.GID(matchData.GID))

	// Check flags.
	if matchData.Match&linux.XT_OWNER_UID != 0 {
		owner.matchUID = true
		if matchData.Invert&linux.XT_OWNER_UID != 0 {
			owner.invertUID = true
		}
	}
	if matchData.Match&linux.XT_OWNER_GID != 0 {
		owner.matchGID = true
		if matchData.Invert&linux.XT_OWNER_GID != 0 {
			owner.invertGID = true
		}
	}

	return &owner, nil
}

// OwnerMatcher matches against a UID and/or GID.
type OwnerMatcher struct {
	uid       auth.KUID
	gid       auth.KGID
	matchUID  bool
	matchGID  bool
	invertUID bool
	invertGID bool
}

// name implements matcher.name.
func (*OwnerMatcher) name() string {
	return matcherNameOwner
}

// Match implements Matcher.Match.
func (om *OwnerMatcher) Match(hook stack.Hook, pkt stack.PacketBufferPtr, _, _ string) (bool, bool) {
	// Support only for OUTPUT chain.
	if hook != stack.Output {
		return false, true
	}

	// If the packet owner is not set, drop the packet.
	if pkt.Owner == nil {
		return false, true
	}

	var matches bool
	// Check for UID match.
	if om.matchUID {
		if auth.KUID(pkt.Owner.KUID()) == om.uid {
			matches = true
		}
		if matches == om.invertUID {
			return false, false
		}
	}

	// Check for GID match.
	if om.matchGID {
		matches = false
		if auth.KGID(pkt.Owner.KGID()) == om.gid {
			matches = true
		}
		if matches == om.invertGID {
			return false, false
		}
	}

	return true, false
}
