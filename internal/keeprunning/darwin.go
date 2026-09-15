//go:build darwin

// Portions of this file are derived from Pulumi's pkg/util/nosleep
// (https://github.com/pulumi/pulumi/tree/master/pkg/util/nosleep), Copyright
// 2016-2024, Pulumi Corporation, licensed under the Apache License, Version
// 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keeprunning

import (
	"context"
	"os"
	"os/exec"
	"strconv"

	"github.com/nanjj/clog"
)

// keepRunning spawns `caffeinate -i -w <pid>` to keep the system awake while
// this process runs. `-i` prevents idle sleep, `-w <pid>` makes caffeinate
// exit automatically when the given process terminates. The acquired lock
// and its release are reported through clog on ctx's span.
func keepRunning(ctx context.Context) DoneFunc {
	cmd := exec.Command("caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		clog.Debug(ctx, "keeprunning: caffeinate unavailable, screen may idle-lock during long run", "err", err)
		return func() {}
	}
	clog.Debug(ctx, "keeprunning: caffeinate keeping system awake")

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		if err := cmd.Process.Kill(); err != nil {
			clog.Debug(ctx, "keeprunning: failed to stop caffeinate", "err", err)
		} else {
			clog.Debug(ctx, "keeprunning: stopped caffeinate")
		}
		_ = cmd.Wait()
	}
}
