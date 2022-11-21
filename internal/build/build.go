// Copyright 2019-2022 Jeroen Simonetti
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package build

import "fmt"

var (
	// Variables populated by linker flags.
	linkVersion string
)

// Banner produces a string banner containing metadata about the currently
// running binary.
func Banner(name string) string {
	// Use n/a as a placeholder if no time set.
	return fmt.Sprintf("%s %s",
		name,
		Version(),
	)
}

// Version produces a Version string or "development" if none was specified
// at link-time.
func Version() string {
	if linkVersion == "" {
		return "development"
	}

	return linkVersion
}
