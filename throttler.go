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

// SetLoggingLevel sets the logging level for the whole application. The values
// accepted are: "debug", "info", "warn" (or "warning"), "error", "fatal" and
// "panic".
func SetLoggingLevel(l string) error {
	levelStr := l

	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		return err
	}

	logrus.SetLevel(level)
	return nil
}

type ksmThrottler struct {
	k   *ksm
	uri string
}

// Kick is the KSM Throttler gRPC Kick function implementation
func (t *ksmThrottler) Kick(context.Context, *gpb.Empty) (*gpb.Empty, error) {
	logrus.Debug("Kick received")

	if t.k == nil {
		return nil, errKSMMissing
	}

	t.k.kick()

	return nil, nil
}

func (t *ksmThrottler) listen() (*net.UnixListener, error) {
	if err := os.Remove(t.uri); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("Couldn't remove exiting socket %v", err)
	}

	listen, err := net.ListenUnix("unix", &net.UnixAddr{Name: t.uri, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("Listen error %v", err)

	}

	if err := os.Chmod(t.uri, 0660|os.ModeSocket); err != nil {
		return nil, fmt.Errorf("Couldn't set mode on socket %v", err)
	}

	return listen, nil
}

func main() {
	ksm, err := startKSM(defaultKSMRoot, defaultKSMMode)
	if err != nil {
		// KSM failure should not be fatal
		logrus.Errorf("Could not start KSM: %v", err)
	}

	throttler := &ksmThrottler{
		k:   ksm,
		uri: defaultgRPCSocket,
	}

	logrus.Debugf("Starting KSM throttling service at %s", throttler.uri)

	listen, err := throttler.listen()
	if err != nil {
		logrus.Errorf("Could not listen on gRPC service %v", err)
	}

	server := grpc.NewServer()
	kpb.RegisterKSMThrottlerServer(server, throttler)

	if err := server.Serve(listen); err != nil {
		logrus.Errorf("gRPC serve error %v\n", err)
		return
	}

	return
}
