//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/kata-containers/ksm-throttler/pkg/client"
	"github.com/sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
	"sync"
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

func waitForDirectory(dir string) error {
	if dir == "" || !path.IsAbs(dir) {
		return fmt.Errorf("Can not wait for empty or relative directory")
	}

	syncCh := make(chan error)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("could not create new watcher %v", err)
		return err
	}
	defer watcher.Close()

	logrus.Debugf("Waiting for %s", dir)

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logrus.Debugf("Directory monitoring event %v", event)
				if event.Op&fsnotify.Create != fsnotify.Create ||
					event.Name != dir {
					continue
				}

				syncCh <- nil
				return

			case err := <-watcher.Errors:
				logrus.Errorf("Directory monitoring error %v", err)
				syncCh <- err
				return
			}
		}
	}()

	if err := watcher.Add(filepath.Dir(dir)); err != nil {
		logrus.Errorf("Could not monitor directory %s: %v", dir, err)
		return err
	}

	select {
	case syncErr := <-syncCh:
		if syncErr == nil {
			logrus.Debugf("%s created", dir)
		}
		return err
	}
}

func monitorPods(vcRunRoot, throttler string) error {
	var wg sync.WaitGroup

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("could not create new watcher %v", err)
		return err
	}
	defer watcher.Close()

	// Wait for vc root if it does not exist
	if _, err := os.Stat(vcRunRoot); os.IsNotExist(err) {
		if err := waitForDirectory(vcRunRoot); err != nil {
			logrus.Errorf("Could not monitor virtcontainers base directory %s: %v", vcRunRoot, err)
			return err
		}
	}

	// Wait for the pods path if it does not exist
	podsPath := filepath.Join(vcRunRoot, "pods")
	if _, err := os.Stat(podsPath); os.IsNotExist(err) {
		if err := waitForDirectory(podsPath); err != nil {
			logrus.Errorf("Could not monitor virtcontainers pods %s: %v", podsPath, err)
			return err
		}

		// First pod created, we should kick the throttler
		if err := client.Kick(throttler); err != nil {
			logrus.Errorf("Could not kick the throttler %v", err)
			return err
		}
	}

	wg.Add(1)

	logrus.Debugf("Monitoring virtcontainers events at %s", podsPath)
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

	if err := watcher.Add(podsPath); err != nil {
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
	vcRoot := flag.String("root", "/var/run/virtcontainers", "Virtcontainers root directory")
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
