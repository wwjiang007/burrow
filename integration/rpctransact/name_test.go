// +build integration

package rpctransact

import (
	"context"
	"testing"

	"github.com/hyperledger/burrow/integration"

	"github.com/hyperledger/burrow/event"
	"github.com/hyperledger/burrow/execution/exec"
	"github.com/hyperledger/burrow/execution/names"
	"github.com/hyperledger/burrow/integration/rpctest"
	"github.com/hyperledger/burrow/rpc/rpcquery"
	"github.com/hyperledger/burrow/txs/payload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameTx(t *testing.T) {
	t.Parallel()
	kern, shutdown := integration.RunNode(t, rpctest.GenesisDoc, rpctest.PrivateAccounts)
	defer shutdown()

	waitNBlocks := func(n uint64) {
		subID := event.GenSubID()
		ch, err := kern.Emitter.Subscribe(context.Background(), subID, exec.QueryForBlockExecution(), 10)
		require.NoError(t, err)
		defer kern.Emitter.UnsubscribeAll(context.Background(), subID)
		for i := uint64(0); i < n; i++ {
			<-ch
		}
	}

	t.Run("Sync", func(t *testing.T) {
		cli := rpctest.NewTransactClient(t, kern.GRPCListenAddress().String())
		name := "Flub"
		data := "floooo"
		expiresIn := uint64(100)
		_, err := cli.NameTxSync(context.Background(), &payload.NameTx{
			Input: &payload.TxInput{
				Address: inputAddress,
				Amount:  names.NameCostForExpiryIn(name, data, expiresIn),
			},
			Name: name,
			Data: data,
		})
		require.NoError(t, err)

		qcli := rpctest.NewQueryClient(t, kern.GRPCListenAddress().String())
		entry, err := qcli.GetName(context.Background(), &rpcquery.GetNameParam{
			Name: "n'existe pas",
		})
		require.Error(t, err)
		entry, err = qcli.GetName(context.Background(), &rpcquery.GetNameParam{
			Name: name,
		})
		require.NoError(t, err)
		assert.Equal(t, name, entry.Name)
		assert.Equal(t, data, entry.Data)
		assert.Equal(t, inputAddress, entry.Owner)
		assert.True(t, entry.Expires >= expiresIn, "expiry should be later than expiresIn")

	})

	t.Run("Registry", func(t *testing.T) {
		tcli := rpctest.NewTransactClient(t, kern.GRPCListenAddress().String())
		qcli := rpctest.NewQueryClient(t, kern.GRPCListenAddress().String())
		names.MinNameRegistrationPeriod = 1

		// register a new name, check if its there
		// since entries ought to be unique and these run against different clients, we append the client
		name := "ye_old_domain_name"
		const data = "if not now, when"
		numDesiredBlocks := uint64(2)

		txe, err := rpctest.UpdateName(tcli, inputAddress, name, data, numDesiredBlocks)
		require.NoError(t, err)

		entry := txe.Result.NameEntry
		assert.NotNil(t, entry, "name should return")
		_, ok := txe.Envelope.Tx.Payload.(*payload.NameTx)
		require.True(t, ok, "should be NameTx: %v", txe.Envelope.Tx.Payload)

		assert.Equal(t, name, entry.Name)
		assert.Equal(t, data, entry.Data)

		entryQuery, err := qcli.GetName(context.Background(), &rpcquery.GetNameParam{Name: name})
		require.NoError(t, err)

		assert.Equal(t, entry, entryQuery)

		// update the data as the owner, make sure still there
		numDesiredBlocks = uint64(3)
		const updatedData = "these are amongst the things I wish to bestow upon " +
			"the youth of generations come: a safe supply of honey, and a better " +
			"money. For what else shall they need"
		_, err = rpctest.UpdateName(tcli, inputAddress, name, updatedData, numDesiredBlocks)
		require.NoError(t, err)

		entry, err = qcli.GetName(context.Background(), &rpcquery.GetNameParam{Name: name})
		require.NoError(t, err)

		assert.Equal(t, updatedData, entry.Data)

		// try to update as non owner, should fail
		txe, err = tcli.NameTxSync(context.Background(), &payload.NameTx{
			Input: &payload.TxInput{
				Address: rpctest.PrivateAccounts[1].GetAddress(),
				Amount:  names.NameCostForExpiryIn(name, data, numDesiredBlocks),
			},
			Name: name,
			Data: "flub flub floo",
		})
		require.Error(t, err, "updating as non-owner on non-expired name should fail")
		assert.Contains(t, err.Error(), "permission denied")

		waitNBlocks(numDesiredBlocks)
		//now the entry should be expired, so we can update as non owner
		const data2 = "this is not my beautiful house"
		owner := rpctest.PrivateAccounts[3].GetAddress()
		txe, err = rpctest.UpdateName(tcli, owner, name, data2, numDesiredBlocks)
		require.NoError(t, err)
		entry = txe.Result.NameEntry

		entryQuery, err = qcli.GetName(context.Background(), &rpcquery.GetNameParam{Name: name})
		require.NoError(t, err)
		assert.Equal(t, entry, entryQuery)
		assert.Equal(t, data2, entry.Data)
		assert.Equal(t, owner, entry.Owner)
	})

}
