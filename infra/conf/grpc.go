package conf

import (
	"google.golang.org/protobuf/proto"

	"github.com/xtls/xray-core/transport/internet/grpc"
)

type GRPCConfig struct {
	ServiceName string `json:"serviceName"`
}

func (g GRPCConfig) Build() (proto.Message, error) {
	return &grpc.Config{ServiceName: g.ServiceName}, nil
}