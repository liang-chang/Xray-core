// +build !confonly

package grpc

import (
	"context"
	gonet "net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/grpc/encoding"
	"github.com/xtls/xray-core/transport/internet/tls"
)

func Dial(ctx context.Context, dest net.Destination, streamSettings *internet.MemoryStreamConfig) (internet.Connection, error) {
	newError("creating connection to ", dest).WriteToLog(session.ExportIDToError(ctx))

	conn, err := dialgRPC(ctx, dest, streamSettings)
	if err != nil {
		return nil, newError("failed to dial gRPC").Base(err)
	}
	return internet.Connection(conn), nil
}

func init() {
	common.Must(internet.RegisterTransportDialer(protocolName, Dial))
}

var (
	globalDialerMap    map[net.Destination]*grpc.ClientConn
	globalDialerAccess sync.Mutex
)

func dialgRPC(ctx context.Context, dest net.Destination, streamSettings *internet.MemoryStreamConfig) (net.Conn, error) {
	grpcSettings := streamSettings.ProtocolSettings.(*Config)

	config := tls.ConfigFromStreamSettings(streamSettings)
	var dialOption = grpc.WithInsecure()

	if config != nil {
		dialOption = grpc.WithTransportCredentials(credentials.NewTLS(config.GetTLSConfig()))
	}

	conn, err := getGrpcClient(dest, dialOption)

	if err != nil {
		return nil, newError("Cannot dial gRPC").Base(err)
	}
	client := encoding.NewGRPCServiceClient(conn)
	grpcservice, err := client.(encoding.GRPCServiceClientX).TunCustomName(ctx, grpcSettings.ServiceName)
	if err != nil {
		return nil, newError("Cannot dial gRPC").Base(err)
	}
	return encoding.NewClientConn(grpcservice), nil
}

func getGrpcClient(dest net.Destination, dialOption grpc.DialOption) (*grpc.ClientConn, error) {
	globalDialerAccess.Lock()
	defer globalDialerAccess.Unlock()

	if globalDialerMap == nil {
		globalDialerMap = make(map[net.Destination]*grpc.ClientConn)
	}

	if client, found := globalDialerMap[dest]; found && client.GetState() != connectivity.Shutdown {
		return client, nil
	}

	conn, err := grpc.Dial(
		dest.Address.String()+":"+dest.Port.String(),
		dialOption,
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  500 * time.Millisecond,
				Multiplier: 1.5,
				Jitter:     0.2,
				MaxDelay:   19 * time.Millisecond,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
		grpc.WithContextDialer(func(ctx context.Context, s string) (gonet.Conn, error) {
			rawHost, rawPort, err := net.SplitHostPort(s)
			if err != nil {
				return nil, err
			}
			if len(rawPort) == 0 {
				rawPort = "443"
			}
			port, err := net.PortFromString(rawPort)
			if err != nil {
				return nil, err
			}
			address := net.ParseAddress(rawHost)
			return internet.DialSystem(ctx, net.TCPDestination(address, port), nil)
		}),
	)
	globalDialerMap[dest] = conn
	return conn, err
}