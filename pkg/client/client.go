//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package client

import (
	"fmt"
	"net"
	"time"

	gpb "github.com/golang/protobuf/ptypes/empty"
	kpb "github.com/kata-containers/ksm-throttler/pkg/grpc"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Kick sends the gRPC Kick message to a KSM throttler service
func Kick(uri string) error {
	// Set up a connection to the server.
	conn, err := grpc.Dial(uri, grpc.WithInsecure(), grpc.WithTimeout(5*time.Second),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		fmt.Printf("Dial error %v\n", err)
		return err
	}
	defer conn.Close()

	client := kpb.NewKSMThrottlerClient(conn)

	_, err = client.Kick(context.Background(), &gpb.Empty{})
	if err != nil {
		fmt.Printf("kick err %v\n", err)
		return err
	}

	return nil
}
