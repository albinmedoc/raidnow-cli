package upstream

import "time"

// Operation identifies one supported upstream operation. Its wire name and
// safety policy are fixed by the package registry and cannot be supplied by a
// caller.
type Operation uint8

const (
	LineMatchingGetList Operation = iota + 1
	ParList
	LineMatchingList
	LineMatchingCheck
	ParFindFirst
	ChildList
	UserInfoPost
	LineMatchingCreate
	LineMatchingUpdate
	LineMatchingCancel
	ChildPostFast
	GuestRaidDelPost
	GuestReadyPost
	ParPost
	ParDelete
	HostReadyPost
	HostKicksGuestPost
	CookieGet
	MyListGet
	JoinListGet
	KickListGet
	DeleteListGet
	BlockGet
)

// OperationDescriptor is an immutable view of centrally registered operation
// metadata.
type OperationDescriptor struct {
	name             string
	kind             OperationKind
	timeout          time.Duration
	allowEmpty       bool
	allowEmptyAny2xx bool
	allowOpaque      bool
}

func (descriptor OperationDescriptor) Name() string                  { return descriptor.name }
func (descriptor OperationDescriptor) Kind() OperationKind           { return descriptor.kind }
func (descriptor OperationDescriptor) DefaultTimeout() time.Duration { return descriptor.timeout }
func (descriptor OperationDescriptor) AllowsEmptyResponse() bool     { return descriptor.allowEmpty }
func (descriptor OperationDescriptor) allowsEmptyAny2xxResponse() bool {
	return descriptor.allowEmptyAny2xx
}
func (descriptor OperationDescriptor) allowsOpaqueResponse() bool { return descriptor.allowOpaque }

func (operation Operation) Descriptor() (OperationDescriptor, error) {
	switch operation {
	case LineMatchingGetList:
		return OperationDescriptor{name: "line_matching_get_list", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case ParList:
		return OperationDescriptor{name: "par_list", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case LineMatchingList:
		return OperationDescriptor{name: "line_matching_list", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case LineMatchingCheck:
		return OperationDescriptor{name: "line_matching_check", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case ParFindFirst:
		return OperationDescriptor{name: "par_find_first", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case ChildList:
		return OperationDescriptor{name: "child_list", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case UserInfoPost:
		return OperationDescriptor{name: "user_info_post", kind: WriteOperation, timeout: 15 * time.Second, allowEmpty: true}, nil
	case LineMatchingCreate:
		return OperationDescriptor{name: "line_matching_create", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case LineMatchingUpdate:
		return OperationDescriptor{name: "line_matching_update", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case LineMatchingCancel:
		return OperationDescriptor{name: "line_matching_cancel", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case ChildPostFast:
		return OperationDescriptor{name: "child_post_fast", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case GuestRaidDelPost:
		return OperationDescriptor{name: "guest_raid_del_post", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case GuestReadyPost:
		return OperationDescriptor{name: "guest_ready_post", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case ParPost:
		return OperationDescriptor{name: "par_post", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case ParDelete:
		return OperationDescriptor{name: "par_delete", kind: WriteOperation, timeout: 15 * time.Second, allowEmpty: true, allowEmptyAny2xx: true, allowOpaque: true}, nil
	case HostReadyPost:
		return OperationDescriptor{name: "host_ready_post", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case HostKicksGuestPost:
		return OperationDescriptor{name: "host_kicks_guest_post", kind: WriteOperation, timeout: 15 * time.Second}, nil
	case CookieGet:
		return OperationDescriptor{name: "cookie_get", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case MyListGet:
		return OperationDescriptor{name: "mylist_get", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case JoinListGet:
		return OperationDescriptor{name: "joinlist_get", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case KickListGet:
		return OperationDescriptor{name: "kicklist_get", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case DeleteListGet:
		return OperationDescriptor{name: "deletelist_get", kind: ReadOperation, timeout: 10 * time.Second}, nil
	case BlockGet:
		return OperationDescriptor{name: "block_get", kind: ReadOperation, timeout: 10 * time.Second}, nil
	default:
		return OperationDescriptor{}, invalidArguments("Ogiltig upstream-operation.")
	}
}
