// Copyright 2017 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"fmt"
	"os"
	"time"

	"github.com/gyuho/linux-inspect/inspect"
	"github.com/gyuho/linux-inspect/top"
)

// startMetrics starts collecting metrics.
func startMetrics(fs *flags, t *transporterServer) (err error) {
	if fs == nil || t == nil || t.cmd == nil {
		return fmt.Errorf("cannot find process to track (%+v, %+v)", fs, t)
	}
	plog.Infof("starting collecting metrics [database %q | PID: %d | disk device: %q | network interface: %q]",
		t.req.DatabaseID, t.pid, fs.diskDevice, fs.networkInterface)

	if err = os.RemoveAll(fs.systemMetricsCSV); err != nil {
		return err
	}
	if err = toFile(fmt.Sprintf("%d", t.req.CurrentClientNumber), t.clientNumPath); err != nil {
		return err
	}

	tcfg := &top.Config{
		Exec:           top.DefaultExecPath,
		IntervalSecond: 1,
		PID:            t.pid,
	}
	t.metricsCSV, err = inspect.NewCSV(
		fs.systemMetricsCSV,
		t.pid,
		fs.diskDevice,
		fs.networkInterface,
		t.clientNumPath,
		tcfg,
	)
	if err := t.metricsCSV.Add(); err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-time.After(time.Second):
				if err := t.metricsCSV.Add(); err != nil {
					plog.Errorf("inspect.CSV.Add error (%v)", err)
					continue
				}

			case <-t.uploadSig:
				plog.Infof("upload signal received; saving CSV at %q", t.metricsCSV.FilePath)

				if err := t.metricsCSV.Save(); err != nil {
					plog.Errorf("inspect.CSV.Save(%q) error %v", t.metricsCSV.FilePath, err)
				} else {
					plog.Infof("CSV saved at %q", t.metricsCSV.FilePath)
				}

				interpolated, err := t.metricsCSV.Interpolate()
				if err != nil {
					plog.Fatalf("inspect.CSV.Interpolate(%q) failed with %v", t.metricsCSV.FilePath, err)
				}
				interpolated.FilePath = fs.systemMetricsCSVInterpolated
				if err := interpolated.Save(); err != nil {
					plog.Errorf("inspect.CSV.Save(%q) error %v", interpolated.FilePath, err)
				} else {
					plog.Infof("CSV saved at %q", interpolated.FilePath)
				}

				close(t.csvReady)
				return

			case sig := <-t.notifier:
				plog.Infof("signal received %q", sig.String())
				return
			}
		}
	}()
	return nil
}
