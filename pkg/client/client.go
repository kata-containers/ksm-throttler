//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package client

import (
	kpb "github.com/kata-containers/ksm-throttler/pkg/grpc"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Kick sends the gRPC Kick message to a KSM throttler service
func Kick(uri string) error {
	// Set up a connection to the server.
	conn, err := grpc.Dial(uri, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()

	client := kpb.NewKSMThrottlerClient(conn)

	_, err = client.Kick(context.Background(), nil)
	if err != nil {
		return err

	}

	return nil
}
