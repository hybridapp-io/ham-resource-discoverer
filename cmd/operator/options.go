// Copyright 2019 The Kubernetes Authors.
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

package operator

import (
	"os"

	pflag "github.com/spf13/pflag"
)

const (
	envVarClusterName      = "CLUSTERNAME"
	envVarClusterNamespace = "CLUSTERNAMESPACE"
	envVarHubClusterConfig = "HUBCLUSTERCONFIGFILE"
)

// SubscriptionCMDOptions for command line flag parsing
type SubscriptionCMDOptions struct {
	ClusterName           string
	ClusterNamespace      string
	HubConfigFilePathName string
}

var Options = SubscriptionCMDOptions{}

// ProcessFlags parses command line parameters into Options
func ProcessFlags() {
	flag := pflag.CommandLine
	// add flags

	flag.StringVar(
		&Options.HubConfigFilePathName,
		"hub-cluster-configfile",
		os.Getenv(envVarHubClusterConfig),
		"Configuration file pathname to hub kubernetes cluster",
	)

	flag.StringVar(
		&Options.ClusterName,
		"cluster-name",
		os.Getenv(envVarClusterName),
		"Name of this endpoint.",
	)

	flag.StringVar(
		&Options.ClusterNamespace,
		"cluster-namespace",
		os.Getenv(envVarClusterNamespace),
		"Cluster Namespace of this endpoint in hub.",
	)
}
