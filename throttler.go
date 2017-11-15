//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	gpb "github.com/golang/protobuf/ptypes/empty"
	kpb "github.com/kata-containers/ksm-throttler/pkg/grpc"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var defaultKSMRoot = "/sys/kernel/mm/ksm/"
var errKSMUnavailable = errors.New("KSM is unavailable")
var errKSMMissing = errors.New("Missing KSM instance")
var memInfo = "/proc/meminfo"

const (
	ksmRunFile        = "run"
	ksmPagesToScan    = "pages_to_scan"
	ksmSleepMillisec  = "sleep_millisecs"
	ksmStart          = "1"
	ksmStop           = "0"
	defaultKSMMode    = ksmAuto
	defaultgRPCSocket = "/var/run/ksmthrottler.sock"
)

type ksmThrottleInterval struct {
	interval time.Duration
	nextKnob ksmMode
}

var ksmAggressiveInterval = 30 * time.Second
var ksmStandardInterval = 120 * time.Second
var ksmSlowInterval = 120 * time.Second

var ksmThrottleIntervals = map[ksmMode]ksmThrottleInterval{
	ksmAggressive: {
		// From aggressive: move to standard and wait 120s
		interval: ksmStandardInterval,
		nextKnob: ksmStandard,
	},

	ksmStandard: {
		// From standard: move to slow and wait 120s
		interval: ksmSlowInterval,
		nextKnob: ksmSlow,
	},

	ksmSlow: {
		// From slow: move to the initial settings and stop there
		interval: 0,
		nextKnob: ksmInitial,
	},

	// We should never make it here
	ksmInitial: {
		interval: 0, // We stay here unless a new container shows up
	},
}

// throttlerLog is the general logger the KSM throttler.
var throttlerLog = logrus.WithFields(logrus.Fields{
	"source": "throttler",
	"name":   "KSM throttler",
	"pid":    os.Getpid(),
})

type ksmThrottler struct {
	k *ksm
}

// Kick is the KSM Throttler gRPC Kick function implementation
func (t *ksmThrottler) Kick(context.Context, *gpb.Empty) (*gpb.Empty, error) {
	if t.k == nil {
		return nil, errKSMMissing
	}

	t.k.kick()

	return nil, nil
}

func main() {
	ksm, err := startKSM(defaultKSMRoot, defaultKSMMode)
	if err != nil {
		// KSM failure should not be fatal
		fmt.Fprintln(os.Stderr, "init:", err.Error())
	} else {
		defer func() {
			_ = ksm.restore()
		}()
	}

	throttler := &ksmThrottler{
		k: ksm,
	}

	listen, err := net.Listen("unix", defaultgRPCSocket)
	if err != nil {
		return
	}

	server := grpc.NewServer()
	kpb.RegisterKSMThrottlerServer(server, throttler)
	server.Serve(listen)
	return
}
