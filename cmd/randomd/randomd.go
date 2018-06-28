// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Binary random carries entropy across restarts.
package main

import (
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"router7/internal/teelogger"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

var log = teelogger.NewConsole()

func seed() error {
	sf, err := os.OpenFile("/perm/random.seed", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer sf.Close()

	rf, err := os.OpenFile("/dev/urandom", os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer rf.Close()

	seed := make([]byte, 512)
	if _, err := sf.Read(seed); err != nil {
		// fall back to seeding with the current time
		seed = []byte(time.Now().String())
	} else {
		log.Printf("seeding RNG from /perm/random.seed")
	}

	entropy := struct {
		entropyCount int64
		bufSize      int64
		buf          [512]byte
	}{
		entropyCount: int64(len(seed) * 8),
		bufSize:      int64(len(seed)),
	}
	copy(entropy.buf[:], seed)
	const RNDADDENTROPY = 0x40085203
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, rf.Fd(), RNDADDENTROPY, uintptr(unsafe.Pointer(&entropy))); errno != 0 {
		return errno
	}

	// Overwrite the seed (re-using a seed compromises security):
	if _, err := sf.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if _, err := io.CopyN(sf, rf, 512); err != nil {
		return err
	}

	return sf.Close()
}

func saveSeed() error {
	rf, err := os.OpenFile("/dev/urandom", os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer rf.Close()

	seed := make([]byte, 512)
	if _, err := rf.Read(seed); err != nil {
		return err
	}
	return ioutil.WriteFile("/perm/random.seed", seed, 0600)
}

func main() {
	if err := seed(); err != nil {
		log.Printf("seeding random pool failed: %v", err)
	}

	// Wait for SIGTERM to store new randomness seed on /perm
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	<-ch

	if err := saveSeed(); err != nil {
		log.Printf("persisting random seed failed: %v", err)
	}
}
