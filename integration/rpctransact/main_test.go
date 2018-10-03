// +build integration

// Space above here matters
// Copyright 2017 Monax Industries Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpctransact

import (
	"context"
	"os"
	"testing"

	"github.com/hyperledger/burrow/core"
	"github.com/hyperledger/burrow/integration"
	"github.com/hyperledger/burrow/integration/rpctest"
	"github.com/hyperledger/burrow/logging/logconfig"
)

var _ = integration.ClaimPorts()
var testConfig = integration.NewTestConfig(rpctest.GenesisDoc)
var kern *core.Kernel

// Needs to be in a _test.go file to be picked up
func TestMain(m *testing.M) {
	cleanup := integration.EnterTestDirectory()
	defer cleanup()
	kern = integration.TestKernel(rpctest.PrivateAccounts[0], rpctest.PrivateAccounts, testConfig,
		logconfig.New().Root(func(sink *logconfig.SinkConfig) *logconfig.SinkConfig {
			return sink
			// Uncomment for debug opcode output
			//return sink.
			//	SetTransform(logconfig.FilterTransform(logconfig.IncludeWhenAllMatch, "tag", "DebugOpcodes")).
			//	SetOutput(logconfig.StdoutOutput().SetFormat("{{.message}}"))
		}))
	err := kern.Boot()
	if err != nil {
		panic(err)
	}
	// Sometimes better to not shutdown as logging errors on shutdown may obscure real issue
	defer func() {
		kern.Shutdown(context.Background())
	}()
	os.Exit(m.Run())
}
