/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Agent watches a path and automatically loads the config stored
// therein.
type Agent struct {
	sync.Mutex
	c *Config
}

// Start will begin polling the config file at the path. If the first load
// fails, Start with return the error and abort. Future load failures will log
// the failure message but continue attempting to load.
func (ca *Agent) Start(path string) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	ca.c = c
	go func() {
		var lastModTime time.Time
		// Rarely, if two changes happen in the same second, mtime will
		// be the same for the second change, and an mtime-based check would
		// fail. Reload periodically just in case.
		skips := 0
		for range time.Tick(1 * time.Second) {
			if skips < 600 {
				// Check if the file changed to see if it needs to be re-read.
				// os.Stat follows symbolic links, which is how ConfigMaps work.
				stat, err := os.Stat(path)
				if err != nil {
					logrus.WithField("path", path).WithError(err).Error("Error loading config.")
					continue
				}
				if stat.ModTime().Equal(lastModTime) {
					skips++
					continue // file hasn't been modified
				}
				lastModTime = stat.ModTime()
			}
			if c, err := Load(path); err != nil {
				logrus.WithField("path", path).WithError(err).Error("Error loading config.")
			} else {
				skips = 0
				ca.Lock()
				ca.c = c
				ca.Unlock()
			}
		}
	}()
	return nil
}

// Config returns the latest config. Do not modify the config.
func (ca *Agent) Config() *Config {
	ca.Lock()
	defer ca.Unlock()
	return ca.c
}

// Set sets the config. Useful for testing.
func (ca *Agent) Set(c *Config) {
	ca.Lock()
	defer ca.Unlock()
	ca.c = c
}
