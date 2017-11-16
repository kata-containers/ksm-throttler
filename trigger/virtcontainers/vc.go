//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/kata-containers/ksm-throttler/pkg/client"
	"github.com/sirupsen/logrus"
)

// DefaultURI is populated at link time with the value of:
//   ${locatestatedir}/run/ksm-throttler/ksm.sock
var DefaultURI string

// ArgURI is populated at runtime from the option -uri
var ArgURI = flag.String("uri", "", "KSM throttler gRPC URI")

const (
	defaultgRPCSocket = "/var/run/ksm-throttler/ksm.sock"
	// In linux the max socket path is 108 including null character
	// see http://man7.org/linux/man-pages/man7/unix.7.html
	socketPathMaxLength = 107
)

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

func monitorPods(vcRunRoot, throttler string) error {
	var wg sync.WaitGroup

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("could not create new watcher %v", err)
		return err
	}
	defer watcher.Close()

	wg.Add(1)

	logrus.Debugf("Monitoring virtcontainers event at %s", vcRunRoot)

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logrus.Debugf("Virtcontainers monitoring event %v", event)
				if event.Op&fsnotify.Create != fsnotify.Create {
					continue
				}

				logrus.Debugf("Kicking KSM throttler at %s", throttler)
				if err := client.Kick(throttler); err != nil {
					logrus.Errorf("Could not kick the throttler %v", err)
					continue
				}

			case err := <-watcher.Errors:
				logrus.Errorf("Virtcontainers monitoring error %v", err)
				wg.Done()
				return
			}
		}
	}()

	if err = watcher.Add(vcRunRoot); err != nil {
		logrus.Errorf("Could not monitor virtcontainers root %s: %v", vcRunRoot, err)
		return err
	}

	wg.Wait()

	return nil
}

func setLoggingLevel(l string) error {
	levelStr := l

	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		return err
	}

	logrus.SetLevel(level)
	return nil
}

func main() {
	vcRoot := flag.String("root", "/var/run/virtcontainers/pods", "Virtcontainers root directory")
	logLevel := flag.String("log", "warn",
		"log messages above specified level; one of debug, warn, error, fatal or panic")
	flag.Parse()

	if err := setLoggingLevel(*logLevel); err != nil {
		fmt.Printf("Could not set logging level %s: %v", *logLevel, err)
	}

	uri, err := getSocketPath()
	if err != nil {
		logrus.Errorf("Could net get service socket URI %v", err)
		return
	}

	if err := monitorPods(*vcRoot, uri); err != nil {
		logrus.Errorf("Could not monitor pods %v", err)
	}
}
