//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	gpb "github.com/golang/protobuf/ptypes/empty"
	kpb "github.com/kata-containers/ksm-throttler/pkg/grpc"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// name describes the program ans is set at build time
var name string

var defaultKSMRoot = "/sys/kernel/mm/ksm/"
var errKSMUnavailable = errors.New("KSM is unavailable")
var errKSMMissing = errors.New("Missing KSM instance")
var memInfo = "/proc/meminfo"

// version is the KSM throttler version. This variable is populated at build time.
var version = "unknown"

// DefaultURI is populated at link time with the value of:
//   ${locatestatedir}/run/ksm-throttler/ksm.sock
var DefaultURI string

// ArgURI is populated at runtime from the option -uri
var ArgURI = flag.String("uri", "", "KSM throttler gRPC URI")

var socketDirectoryPerm = os.FileMode(0750)

const (
	ksmRunFile        = "run"
	ksmPagesToScan    = "pages_to_scan"
	ksmSleepMillisec  = "sleep_millisecs"
	ksmStart          = "1"
	ksmStop           = "0"
	defaultKSMMode    = ksmAuto
	defaultgRPCSocket = "/var/run/ksm-throttler/ksm.sock"
	// In linux the max socket path is 108 including null character
	// see http://man7.org/linux/man-pages/man7/unix.7.html
	socketPathMaxLength = 107
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
	"name":   name,
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

	throttlerLog.WithField("version", version).Info()

	return nil
}

type ksmThrottler struct {
	k   *ksm
	uri string
}

// Kick is the KSM Throttler gRPC Kick function implementation
func (t *ksmThrottler) Kick(context.Context, *gpb.Empty) (*gpb.Empty, error) {
	throttlerLog.Debug("Kick received")

	if t.k == nil {
		return nil, errKSMMissing
	}

	t.k.kick()

	return &gpb.Empty{}, nil
}

func (t *ksmThrottler) listen() (*net.UnixListener, error) {
	uriDir := filepath.Dir(t.uri)
	if err := os.MkdirAll(uriDir, socketDirectoryPerm); err != nil {
		return nil, fmt.Errorf("Couldn't create socket directory %v", err)
	}

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

// getSocketPath computes the path of the KSM throttler socket.
// Note that when socket activated, the socket path is specified
// in the systemd socket file but the same value is set in
// DefaultURI at link time.
func getSocketPath() (string, error) {
	// Invoking "go build" without any linker option will not
	// populate DefaultURI, so fallback to a reasonable
	// path. People should really use the Makefile though.
	if DefaultURI == "" {
		DefaultURI = defaultgRPCSocket
	}

	socketURI := DefaultURI

	if len(*ArgURI) != 0 {
		socketURI = *ArgURI
	}

	if len(socketURI) > socketPathMaxLength {
		return "", fmt.Errorf("socket path too long %d (max %d)",
			len(socketURI), socketPathMaxLength)

	}

	return socketURI, nil
}

func main() {
	doVersion := flag.Bool("version", false, "display the version")
	logLevel := flag.String("log", "warn",
		"log messages above specified level; one of debug, warn, error, fatal or panic")

	flag.Parse()

	uri, err := getSocketPath()
	if err != nil {
		logrus.Errorf("Could net get service socket URI: %v", err)
		return
	}

	if err := SetLoggingLevel(*logLevel); err != nil {
		fmt.Printf("Could not set logging level %s: %v", *logLevel, err)
	}

	if *doVersion {
		fmt.Printf("%v version %v\n", name, version)
		os.Exit(0)
	}

	ksm, err := startKSM(defaultKSMRoot, defaultKSMMode)
	if err != nil {
		logrus.Errorf("Could not start KSM: %v", err)
		return
	}

	throttler := &ksmThrottler{
		k:   ksm,
		uri: uri,
	}

	throttlerLog.Debugf("Starting KSM throttling service at %s", throttler.uri)

	listen, err := throttler.listen()
	if err != nil {
		throttlerLog.Errorf("Could not listen on gRPC service %v", err)
	}

	server := grpc.NewServer()
	kpb.RegisterKSMThrottlerServer(server, throttler)

	if err := server.Serve(listen); err != nil {
		throttlerLog.Errorf("gRPC serve error %v", err)
		return
	}

	return
}
