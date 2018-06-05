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
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kata-containers/ksm-throttler/pkg/client"
	ksig "github.com/kata-containers/ksm-throttler/pkg/signals"
	"github.com/sirupsen/logrus"
)

// DefaultURI is populated at link time with the value of:
//   ${locatestatedir}/run/ksm-throttler/ksm.sock
var DefaultURI string

// ArgURI is populated at runtime from the option -uri
var ArgURI = flag.String("uri", "", "KSM throttler gRPC URI")

var triggerLog = logrus.WithFields(logrus.Fields{
	"source": "throttler-trigger",
	"name":   "vc",
	"pid":    os.Getpid(),
})

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

	logger := triggerLog.WithField("directory", dir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.WithError(err).Error("could not create new watcher")
		return err
	}
	defer watcher.Close()

	logger.Debug("Waiting for directory")

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logger.WithField("event", event).Debug("Got event")
				if event.Op&fsnotify.Create != fsnotify.Create ||
					event.Name != dir {
					continue
				}

				syncCh <- nil
				return

			case err := <-watcher.Errors:
				logger.WithError(err).Error("Directory monitoring failed")
				syncCh <- err
				return
			}
		}
	}()

	if err := watcher.Add(filepath.Dir(dir)); err != nil {
		logger.WithError(err).Error("Could not monitor directory")
		return err
	}

	select {
	case syncErr := <-syncCh:
		if syncErr == nil {
			logger.Debug("directory created")
		}
		return err
	}
}

func monitorPods(vcRunRoot, throttler string) error {
	var wg sync.WaitGroup

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		triggerLog.WithError(err).Error("could not create new watcher")
		return err
	}
	defer watcher.Close()

	logger := triggerLog.WithFields(logrus.Fields{
		"vc-root":   vcRunRoot,
		"throttler": throttler,
	})

	// Wait for vc root if it does not exist
	if _, err := os.Stat(vcRunRoot); os.IsNotExist(err) {
		if err := waitForDirectory(vcRunRoot); err != nil {
			logger.WithError(err).Error("Could not monitor virtcontainers base directory")
			return err
		}
	}

	// Wait for the pods path if it does not exist
	podsPath := filepath.Join(vcRunRoot, "sbs")

	logger = logger.WithField("pods-path", podsPath)

	if _, err := os.Stat(podsPath); os.IsNotExist(err) {
		if err := waitForDirectory(podsPath); err != nil {
			logger.WithError(err).Error("Could not monitor virtcontainers pods")
			return err
		}

		// First pod created, we should kick the throttler
		if err := client.Kick(throttler); err != nil {
			logger.WithError(err).Error("Could not kick the throttler")
			return err
		}
	}

	wg.Add(1)

	logger.Debug("Monitoring virtcontainers events")

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logger.WithField("event", event).Debug("Virtcontainers monitoring event")
				if event.Op&fsnotify.Create != fsnotify.Create {
					continue
				}

				logger.Debug("Kicking KSM throttler")
				if err := client.Kick(throttler); err != nil {
					logger.WithError(err).Error("Could not kick the throttler")
					continue
				}

			case err := <-watcher.Errors:
				logger.WithError(err).Error("Virtcontainers monitoring error")
				wg.Done()
				return
			}
		}
	}()

	if err := watcher.Add(podsPath); err != nil {
		logger.WithError(err).Error("Could not monitor virtcontainers root")
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

	triggerLog.Logger.SetLevel(level)

	triggerLog.Logger.Formatter = &logrus.TextFormatter{TimestampFormat: time.RFC3339Nano}

	return nil
}

func setupSignalHandler() {
	sigCh := make(chan os.Signal, 8)

	for _, sig := range ksig.HandledSignals() {
		signal.Notify(sigCh, sig)
	}

	var debug = true

	go func() {
		for {
			sig := <-sigCh

			nativeSignal, ok := sig.(syscall.Signal)
			if !ok {
				err := errors.New("unknown signal")
				triggerLog.WithError(err).WithField("signal", sig.String()).Error()
				continue
			}

			if ksig.FatalSignal(nativeSignal) {
				triggerLog.WithField("signal", sig).Error("received fatal signal")
				ksig.Die()
			} else if ksig.NonFatalSignal(nativeSignal) {
				if debug {
					triggerLog.WithField("signal", sig).Debug("handling signal")
					ksig.Backtrace()
				}
			}
		}
	}()
}

func main() {
	vcRoot := flag.String("root", "/var/run/virtcontainers", "Virtcontainers root directory")
	logLevel := flag.String("log", "warn",
		"log messages above specified level; one of debug, warn, error, fatal or panic")
	flag.Parse()

	if err := setLoggingLevel(*logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Could not set logging level %s: %v", *logLevel, err)
		os.Exit(1)
	}

	ksig.SetLogger(triggerLog)

	uri, err := getSocketPath()
	if err != nil {
		logrus.WithError(err).Error("Could net get service socket URI")
		os.Exit(1)
	}

	setupSignalHandler()

	if err := monitorPods(*vcRoot, uri); err != nil {
		logrus.WithError(err).Error("Could not monitor pods")
		os.Exit(1)
	}
}
