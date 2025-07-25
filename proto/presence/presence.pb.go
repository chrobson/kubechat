// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v5.29.3
// source: proto/presence.proto

package presence

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type SetUserOnlineRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserId        string                 `protobuf:"bytes,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetUserOnlineRequest) Reset() {
	*x = SetUserOnlineRequest{}
	mi := &file_proto_presence_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetUserOnlineRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetUserOnlineRequest) ProtoMessage() {}

func (x *SetUserOnlineRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetUserOnlineRequest.ProtoReflect.Descriptor instead.
func (*SetUserOnlineRequest) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{0}
}

func (x *SetUserOnlineRequest) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

type SetUserOnlineResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Success       bool                   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message       string                 `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetUserOnlineResponse) Reset() {
	*x = SetUserOnlineResponse{}
	mi := &file_proto_presence_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetUserOnlineResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetUserOnlineResponse) ProtoMessage() {}

func (x *SetUserOnlineResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetUserOnlineResponse.ProtoReflect.Descriptor instead.
func (*SetUserOnlineResponse) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{1}
}

func (x *SetUserOnlineResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *SetUserOnlineResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type SetUserOfflineRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserId        string                 `protobuf:"bytes,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetUserOfflineRequest) Reset() {
	*x = SetUserOfflineRequest{}
	mi := &file_proto_presence_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetUserOfflineRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetUserOfflineRequest) ProtoMessage() {}

func (x *SetUserOfflineRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetUserOfflineRequest.ProtoReflect.Descriptor instead.
func (*SetUserOfflineRequest) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{2}
}

func (x *SetUserOfflineRequest) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

type SetUserOfflineResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Success       bool                   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message       string                 `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetUserOfflineResponse) Reset() {
	*x = SetUserOfflineResponse{}
	mi := &file_proto_presence_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetUserOfflineResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetUserOfflineResponse) ProtoMessage() {}

func (x *SetUserOfflineResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetUserOfflineResponse.ProtoReflect.Descriptor instead.
func (*SetUserOfflineResponse) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{3}
}

func (x *SetUserOfflineResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *SetUserOfflineResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type GetUserStatusRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserId        string                 `protobuf:"bytes,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetUserStatusRequest) Reset() {
	*x = GetUserStatusRequest{}
	mi := &file_proto_presence_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetUserStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetUserStatusRequest) ProtoMessage() {}

func (x *GetUserStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetUserStatusRequest.ProtoReflect.Descriptor instead.
func (*GetUserStatusRequest) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{4}
}

func (x *GetUserStatusRequest) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

type GetUserStatusResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserId        string                 `protobuf:"bytes,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	Online        bool                   `protobuf:"varint,2,opt,name=online,proto3" json:"online,omitempty"`
	LastSeen      *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=last_seen,json=lastSeen,proto3" json:"last_seen,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetUserStatusResponse) Reset() {
	*x = GetUserStatusResponse{}
	mi := &file_proto_presence_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetUserStatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetUserStatusResponse) ProtoMessage() {}

func (x *GetUserStatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetUserStatusResponse.ProtoReflect.Descriptor instead.
func (*GetUserStatusResponse) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{5}
}

func (x *GetUserStatusResponse) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

func (x *GetUserStatusResponse) GetOnline() bool {
	if x != nil {
		return x.Online
	}
	return false
}

func (x *GetUserStatusResponse) GetLastSeen() *timestamppb.Timestamp {
	if x != nil {
		return x.LastSeen
	}
	return nil
}

type GetOnlineUsersRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetOnlineUsersRequest) Reset() {
	*x = GetOnlineUsersRequest{}
	mi := &file_proto_presence_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetOnlineUsersRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetOnlineUsersRequest) ProtoMessage() {}

func (x *GetOnlineUsersRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetOnlineUsersRequest.ProtoReflect.Descriptor instead.
func (*GetOnlineUsersRequest) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{6}
}

type GetOnlineUsersResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserIds       []string               `protobuf:"bytes,1,rep,name=user_ids,json=userIds,proto3" json:"user_ids,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetOnlineUsersResponse) Reset() {
	*x = GetOnlineUsersResponse{}
	mi := &file_proto_presence_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetOnlineUsersResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetOnlineUsersResponse) ProtoMessage() {}

func (x *GetOnlineUsersResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetOnlineUsersResponse.ProtoReflect.Descriptor instead.
func (*GetOnlineUsersResponse) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{7}
}

func (x *GetOnlineUsersResponse) GetUserIds() []string {
	if x != nil {
		return x.UserIds
	}
	return nil
}

type UserStatusEvent struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserId        string                 `protobuf:"bytes,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	Online        bool                   `protobuf:"varint,2,opt,name=online,proto3" json:"online,omitempty"`
	Timestamp     *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *UserStatusEvent) Reset() {
	*x = UserStatusEvent{}
	mi := &file_proto_presence_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UserStatusEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UserStatusEvent) ProtoMessage() {}

func (x *UserStatusEvent) ProtoReflect() protoreflect.Message {
	mi := &file_proto_presence_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UserStatusEvent.ProtoReflect.Descriptor instead.
func (*UserStatusEvent) Descriptor() ([]byte, []int) {
	return file_proto_presence_proto_rawDescGZIP(), []int{8}
}

func (x *UserStatusEvent) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

func (x *UserStatusEvent) GetOnline() bool {
	if x != nil {
		return x.Online
	}
	return false
}

func (x *UserStatusEvent) GetTimestamp() *timestamppb.Timestamp {
	if x != nil {
		return x.Timestamp
	}
	return nil
}

var File_proto_presence_proto protoreflect.FileDescriptor

const file_proto_presence_proto_rawDesc = "" +
	"\n" +
	"\x14proto/presence.proto\x12\bpresence\x1a\x1fgoogle/protobuf/timestamp.proto\"/\n" +
	"\x14SetUserOnlineRequest\x12\x17\n" +
	"\auser_id\x18\x01 \x01(\tR\x06userId\"K\n" +
	"\x15SetUserOnlineResponse\x12\x18\n" +
	"\asuccess\x18\x01 \x01(\bR\asuccess\x12\x18\n" +
	"\amessage\x18\x02 \x01(\tR\amessage\"0\n" +
	"\x15SetUserOfflineRequest\x12\x17\n" +
	"\auser_id\x18\x01 \x01(\tR\x06userId\"L\n" +
	"\x16SetUserOfflineResponse\x12\x18\n" +
	"\asuccess\x18\x01 \x01(\bR\asuccess\x12\x18\n" +
	"\amessage\x18\x02 \x01(\tR\amessage\"/\n" +
	"\x14GetUserStatusRequest\x12\x17\n" +
	"\auser_id\x18\x01 \x01(\tR\x06userId\"\x81\x01\n" +
	"\x15GetUserStatusResponse\x12\x17\n" +
	"\auser_id\x18\x01 \x01(\tR\x06userId\x12\x16\n" +
	"\x06online\x18\x02 \x01(\bR\x06online\x127\n" +
	"\tlast_seen\x18\x03 \x01(\v2\x1a.google.protobuf.TimestampR\blastSeen\"\x17\n" +
	"\x15GetOnlineUsersRequest\"3\n" +
	"\x16GetOnlineUsersResponse\x12\x19\n" +
	"\buser_ids\x18\x01 \x03(\tR\auserIds\"|\n" +
	"\x0fUserStatusEvent\x12\x17\n" +
	"\auser_id\x18\x01 \x01(\tR\x06userId\x12\x16\n" +
	"\x06online\x18\x02 \x01(\bR\x06online\x128\n" +
	"\ttimestamp\x18\x03 \x01(\v2\x1a.google.protobuf.TimestampR\ttimestamp2\xdf\x02\n" +
	"\x0fPresenceService\x12P\n" +
	"\rSetUserOnline\x12\x1e.presence.SetUserOnlineRequest\x1a\x1f.presence.SetUserOnlineResponse\x12S\n" +
	"\x0eSetUserOffline\x12\x1f.presence.SetUserOfflineRequest\x1a .presence.SetUserOfflineResponse\x12P\n" +
	"\rGetUserStatus\x12\x1e.presence.GetUserStatusRequest\x1a\x1f.presence.GetUserStatusResponse\x12S\n" +
	"\x0eGetOnlineUsers\x12\x1f.presence.GetOnlineUsersRequest\x1a .presence.GetOnlineUsersResponseB\x12Z\x10./proto/presenceb\x06proto3"

var (
	file_proto_presence_proto_rawDescOnce sync.Once
	file_proto_presence_proto_rawDescData []byte
)

func file_proto_presence_proto_rawDescGZIP() []byte {
	file_proto_presence_proto_rawDescOnce.Do(func() {
		file_proto_presence_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_proto_presence_proto_rawDesc), len(file_proto_presence_proto_rawDesc)))
	})
	return file_proto_presence_proto_rawDescData
}

var file_proto_presence_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_proto_presence_proto_goTypes = []any{
	(*SetUserOnlineRequest)(nil),   // 0: presence.SetUserOnlineRequest
	(*SetUserOnlineResponse)(nil),  // 1: presence.SetUserOnlineResponse
	(*SetUserOfflineRequest)(nil),  // 2: presence.SetUserOfflineRequest
	(*SetUserOfflineResponse)(nil), // 3: presence.SetUserOfflineResponse
	(*GetUserStatusRequest)(nil),   // 4: presence.GetUserStatusRequest
	(*GetUserStatusResponse)(nil),  // 5: presence.GetUserStatusResponse
	(*GetOnlineUsersRequest)(nil),  // 6: presence.GetOnlineUsersRequest
	(*GetOnlineUsersResponse)(nil), // 7: presence.GetOnlineUsersResponse
	(*UserStatusEvent)(nil),        // 8: presence.UserStatusEvent
	(*timestamppb.Timestamp)(nil),  // 9: google.protobuf.Timestamp
}
var file_proto_presence_proto_depIdxs = []int32{
	9, // 0: presence.GetUserStatusResponse.last_seen:type_name -> google.protobuf.Timestamp
	9, // 1: presence.UserStatusEvent.timestamp:type_name -> google.protobuf.Timestamp
	0, // 2: presence.PresenceService.SetUserOnline:input_type -> presence.SetUserOnlineRequest
	2, // 3: presence.PresenceService.SetUserOffline:input_type -> presence.SetUserOfflineRequest
	4, // 4: presence.PresenceService.GetUserStatus:input_type -> presence.GetUserStatusRequest
	6, // 5: presence.PresenceService.GetOnlineUsers:input_type -> presence.GetOnlineUsersRequest
	1, // 6: presence.PresenceService.SetUserOnline:output_type -> presence.SetUserOnlineResponse
	3, // 7: presence.PresenceService.SetUserOffline:output_type -> presence.SetUserOfflineResponse
	5, // 8: presence.PresenceService.GetUserStatus:output_type -> presence.GetUserStatusResponse
	7, // 9: presence.PresenceService.GetOnlineUsers:output_type -> presence.GetOnlineUsersResponse
	6, // [6:10] is the sub-list for method output_type
	2, // [2:6] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_proto_presence_proto_init() }
func file_proto_presence_proto_init() {
	if File_proto_presence_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_proto_presence_proto_rawDesc), len(file_proto_presence_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_presence_proto_goTypes,
		DependencyIndexes: file_proto_presence_proto_depIdxs,
		MessageInfos:      file_proto_presence_proto_msgTypes,
	}.Build()
	File_proto_presence_proto = out.File
	file_proto_presence_proto_goTypes = nil
	file_proto_presence_proto_depIdxs = nil
}
