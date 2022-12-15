// Code generated by protoc-gen-go-vtproto. DO NOT EDIT.
// protoc-gen-go-vtproto version: v0.3.0
// source: apis/proto/v1/vald/flush.proto

package vald

import (
	context "context"
	payload "github.com/vdaa s/vald/apis/grpc/v1/payload"
	grpc "google.golang.org/grpc"
	codes "github.com/vdaas/vald/internal/net/grpc/codes"
	status "github.com/vdaas/vald/internal/net/grpc/status"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// FlushClient is the client API for Flush service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FlushClient interface {
	// A method to flush all indexed vector.
	Flush(ctx context.Context, in *payload.Flush_Request, opts ...grpc.CallOption) (*payload.Info_Index_Count, error)
}

type flushClient struct {
	cc grpc.ClientConnInterface
}

func NewFlushClient(cc grpc.ClientConnInterface) FlushClient {
	return &flushClient{cc}
}

func (c *flushClient) Flush(ctx context.Context, in *payload.Flush_Request, opts ...grpc.CallOption) (*payload.Info_Index_Count, error) {
	out := new(payload.Info_Index_Count)
	err := c.cc.Invoke(ctx, "/vald.v1.Flush/Flush", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FlushServer is the server API for Flush service.
// All implementations must embed UnimplementedFlushServer
// for forward compatibility
type FlushServer interface {
	// A method to flush all indexed vector.
	Flush(context.Context, *payload.Flush_Request) (*payload.Info_Index_Count, error)
	mustEmbedUnimplementedFlushServer()
}

// UnimplementedFlushServer must be embedded to have forward compatible implementations.
type UnimplementedFlushServer struct {
}

func (UnimplementedFlushServer) Flush(context.Context, *payload.Flush_Request) (*payload.Info_Index_Count, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Flush not implemented")
}
func (UnimplementedFlushServer) mustEmbedUnimplementedFlushServer() {}

// UnsafeFlushServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FlushServer will
// result in compilation errors.
type UnsafeFlushServer interface {
	mustEmbedUnimplementedFlushServer()
}

func RegisterFlushServer(s grpc.ServiceRegistrar, srv FlushServer) {
	s.RegisterService(&Flush_ServiceDesc, srv)
}

func _Flush_Flush_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(payload.Flush_Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FlushServer).Flush(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/vald.v1.Flush/Flush",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FlushServer).Flush(ctx, req.(*payload.Flush_Request))
	}
	return interceptor(ctx, in, info, handler)
}

// Flush_ServiceDesc is the grpc.ServiceDesc for Flush service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Flush_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "vald.v1.Flush",
	HandlerType: (*FlushServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Flush",
			Handler:    _Flush_Flush_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "apis/proto/v1/vald/flush.proto",
}
