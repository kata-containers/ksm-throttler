//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"

	"github.com/kata-containers/ksm-throttler/pkg/client"
)

func main() {
	uri := flag.String("uri", "/var/run/kata-ksm-throttler/ksm.sock", "KSM throttler gRPC URI")
	flag.Parse()

	err := client.Kick(*uri)
	if err != nil {
		fmt.Println(err)
	}
}
