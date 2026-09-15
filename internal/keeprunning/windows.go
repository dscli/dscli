//go:build windows

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
	"golang.org/x/sys/windows"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// keepRunning tells Windows not to put the system to sleep while this process
// runs. ES_CONTINUOUS resets the idle timer continuously; ES_SYSTEM_REQUIRED
// forces the display system to stay on.
func keepRunning() DoneFunc {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	setThreadExecutionState := kernel32.NewProc("SetThreadExecutionState")

	if r, _, _ := setThreadExecutionState.Call(
		uintptr(esContinuous | esSystemRequired),
	); r == 0 {
		return func() {}
	}

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		setThreadExecutionState.Call(uintptr(esContinuous))
	}
}
