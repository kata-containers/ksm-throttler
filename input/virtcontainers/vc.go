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
	uri := flag.String("uri", "/var/run/ksmthrottler.sock", "KSM throttler gRPC URI")
	vcRoot := flag.String("root", "/var/run/virtcontainers/pods", "Virtcontainers root directory")
	logLevel := flag.String("log", "warn",
		"log messages above specified level; one of debug, warn, error, fatal or panic")
	flag.Parse()

	if err := setLoggingLevel(*logLevel); err != nil {
		fmt.Printf("Could not set logging level %s: %v", *logLevel, err)
	}

	if err := monitorPods(*vcRoot, *uri); err != nil {
		logrus.Errorf("Could not monitor pods %v", err)
	}
}
