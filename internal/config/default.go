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
package config

import (
	"fmt"

	"github.com/jsimonetti/hodos/internal/build"
)

const defaultConfig = `# %s configuration file

# global defaults (override per interface or per host)
# debug = false
# icmp_interval = "500ms"
# icmp_timeout = "200ms"
# burst_size = 1
# burst_interval

# command to run at up or down state
# up_action = "/path/to/script"
# down_action = "/path/to/script"

# start monitoring interface eth0 and use routing table 2
[[interfaces]]
name = "eth0"
table = 2
metric = 1000
# debug = false

# amount of hosts that need to be up for this interface to be considered up
# minimum_up = 1

# command to run at up or down state
# up_action = "/path/to/script"
# down_action = "/path/to/script"
# icmp_interval = "500ms"
# icmp_timeout = "200ms"
# burst_size = 1
# burst_interval

[[interfaces.hosts]]
name = "Cloudflare"
host = "1.1.1.1"
# debug = false
# icmp_interval = "500ms"
# icmp_timeout = "200ms"
# burst_size = 1
# burst_interval

[[interfaces.hosts]]
name = "Cloudflare"
host = "2606:4700:4700::1111"

[[interfaces.hosts]]
name = "Google"
host = "8.8.8.8"

[[interfaces.hosts]]
name = "Google"
host = "2001:4860:4860::8888"
`

func DefaulConfig() string {
	return fmt.Sprintf(defaultConfig, build.Banner("hodos"))
}
