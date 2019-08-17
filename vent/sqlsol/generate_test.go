package sqlsol_test

import (
	"testing"

	"github.com/hyperledger/burrow/execution/evm/abi"
	"github.com/hyperledger/burrow/execution/solidity"
	"github.com/hyperledger/burrow/vent/sqlsol"
	"github.com/hyperledger/burrow/vent/types"
	"github.com/stretchr/testify/require"
)

func TestGenerateSpecFromAbis(t *testing.T) {
	spec, err := abi.ReadSpec(solidity.Abi_EventEmitter)
	require.NoError(t, err)

	project, err := sqlsol.GenerateSpecFromAbis(spec)
	require.NoError(t, err)

	require.ElementsMatch(t, project[0].FieldMappings,
		[]*types.EventFieldMapping{
			{
				Field:      "trueism",
				ColumnName: "trueism",
				Type:       "bool",
			},
			{
				Field:      "german",
				ColumnName: "german",
				Type:       "string",
			},
			{
				Field:      "newDepth",
				ColumnName: "newDepth",
				Type:       "int128",
			},
			{
				Field:      "bignum",
				ColumnName: "bignum",
				Type:       "int256",
			},
			{
				Field:      "hash",
				ColumnName: "hash",
				Type:       "bytes32",
			},
			{
				Field:      "direction",
				ColumnName: "direction",
				Type:       "bytes32",
			},
		})
}
