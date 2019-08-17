package execution

import (
	"fmt"

	"github.com/hyperledger/burrow/execution/evm"
)

type VMOption string

const (
	DebugOpcodes VMOption = "DebugOpcodes"
	DumpTokens   VMOption = "DumpTokens"
)

type ExecutionConfig struct {
	// This parameter scales the default Tendermint timeouts. A value of 1 gives the Tendermint defaults designed to
	// work for 100 node + public network. Smaller networks should be able to sustain lower values.
	// When running in no-consensus mode (Tendermint.Enabled = false) this scales the block duration with 1.0 meaning 1 second
	// and 0 meaning commit immediately
	TimeoutFactor            float64
	CallStackMaxDepth        uint64
	DataStackInitialCapacity uint64
	DataStackMaxDepth        uint64
	VMOptions                []VMOption `json:",omitempty" toml:",omitempty"`
}

func DefaultExecutionConfig() *ExecutionConfig {
	return &ExecutionConfig{
		CallStackMaxDepth:        0, // Unlimited by default
		DataStackInitialCapacity: evm.DataStackInitialCapacity,
		DataStackMaxDepth:        0, // Unlimited by default
		TimeoutFactor:            0.33,
	}
}

type ExecutionOption func(*executor)

func VMOptions(vmOptions ...func(*evm.VM)) func(*executor) {
	return func(exe *executor) {
		exe.vmOptions = vmOptions
	}
}

func (ec *ExecutionConfig) ExecutionOptions() ([]ExecutionOption, error) {
	var exeOptions []ExecutionOption
	var vmOptions []func(*evm.VM)
	for _, option := range ec.VMOptions {
		switch option {
		case DebugOpcodes:
			vmOptions = append(vmOptions, evm.DebugOpcodes)
		case DumpTokens:
			vmOptions = append(vmOptions, evm.DumpTokens)
		default:
			return nil, fmt.Errorf("VM option '%s' not recognised", option)
		}
	}
	vmOptions = append(vmOptions, evm.StackOptions(ec.CallStackMaxDepth, ec.DataStackInitialCapacity, ec.DataStackMaxDepth))
	exeOptions = append(exeOptions, VMOptions(vmOptions...))
	return exeOptions, nil
}
